// Package password implements bounded, peppered Argon2id credentials.
// It is not an authentication endpoint: callers must additionally handle account
// enumeration, rate limits, sessions and atomic credential replacement.
package password

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	Scheme           = "argon2id-hmac-sha256-v1"
	MaxPasswordBytes = 1024
	MinPasswordRunes = 15
	saltBytes        = 16
	hashBytes        = 32
	maxPHCBytes      = 256
)

var (
	ErrConfiguration     = errors.New("invalid password hashing configuration")
	ErrPasswordInput     = errors.New("invalid password input")
	ErrCredential        = errors.New("invalid stored password credential")
	ErrPepperUnavailable = errors.New("password verification key unavailable")
	ErrBusy              = errors.New("password hashing capacity exhausted")
	ErrEntropy           = errors.New("password salt generation failed")
)

// Params are public Argon2id parameters; MemoryKiB is measured in KiB.
type Params struct {
	MemoryKiB, Iterations uint32
	Parallelism           uint8
}

func DefaultParams() Params { return Params{MemoryKiB: 64 * 1024, Iterations: 3, Parallelism: 1} }

func (p Params) valid() bool {
	return p.MemoryKiB >= 19*1024 && p.MemoryKiB <= 128*1024 && p.Iterations >= 2 && p.Iterations <= 6 && p.Parallelism >= 1 && p.Parallelism <= 4
}

// Credential fields belong in private database columns, never an API response.
// PHC contains salt, parameters and derived hash; the pepper is never stored here.
type Credential struct {
	Scheme      string `json:"-"`
	PepperKeyID string `json:"-"`
	PHC         string `json:"-"`
}

func (Credential) String() string     { return "password.Credential(redacted)" }
func (c Credential) GoString() string { return c.String() }

// Verification.Replacement is present only after successful verification when
// key/parameter upgrade is required. Persist it with compare-and-swap against the
// verified credential so a concurrent password reset cannot be overwritten.
type Verification struct {
	Match       bool
	Replacement *Credential `json:"-"`
}

// Hasher owns immutable copies of keys and a shared, fail-fast concurrency gate.
// Reuse one instance per process, rather than creating one per login request.
type Hasher struct {
	params Params
	active string
	keys   map[string][]byte
	slots  chan struct{}
}

func (*Hasher) String() string     { return "password.Hasher(redacted)" }
func (h *Hasher) GoString() string { return h.String() }

func validKeyID(id string) bool {
	if len(id) < 1 || len(id) > 32 {
		return false
	}
	for _, c := range []byte(id) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}

// New requires an explicit pepper; there is no unpeppered fallback. Keys must
// contain 32..64 cryptographically random bytes and have non-secret identifiers.
// Retain old keys until their credentials have been migrated or reset.
func New(params Params, active string, keys map[string][]byte, concurrent int) (*Hasher, error) {
	if !params.valid() || !validKeyID(active) || len(keys) < 1 || len(keys) > 8 || concurrent < 1 || concurrent > 2 {
		return nil, ErrConfiguration
	}
	copied := make(map[string][]byte, len(keys))
	for id, key := range keys {
		if !validKeyID(id) || len(key) < 32 || len(key) > 64 {
			return nil, ErrConfiguration
		}
		copied[id] = append([]byte(nil), key...)
	}
	if _, ok := copied[active]; !ok {
		return nil, ErrConfiguration
	}
	return &Hasher{params: params, active: active, keys: copied, slots: make(chan struct{}, concurrent)}, nil
}

// ValidateNewPassword applies the creation length policy. No trimming,
// truncation or Unicode normalization occurs. A registration service must also
// reject known compromised passwords before calling Hash.
func ValidateNewPassword(value string) error {
	if len(value) > MaxPasswordBytes || !utf8.ValidString(value) || utf8.RuneCountInString(value) < MinPasswordRunes {
		return ErrPasswordInput
	}
	return nil
}

func (h *Hasher) acquire(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case h.slots <- struct{}{}:
		return nil
	default:
		return ErrBusy
	}
}

func derive(value string, key, salt []byte, p Params) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("devcourse-finder/password/v1\x00"))
	raw := []byte(value)
	_, _ = mac.Write(raw)
	clear(raw)
	seed := mac.Sum(nil)
	defer clear(seed)
	return argon2.IDKey(seed, salt, p.Iterations, p.MemoryKiB, p.Parallelism, hashBytes)
}

func (h *Hasher) hash(ctx context.Context, value string, p Params) (Credential, error) {
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}
	salt := make([]byte, saltBytes)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return Credential{}, ErrEntropy
	}
	sum := derive(value, h.keys[h.active], salt, p)
	defer clear(sum)
	if err := ctx.Err(); err != nil {
		return Credential{}, err
	}
	phc := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, p.MemoryKiB, p.Iterations, p.Parallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(sum))
	return Credential{Scheme: Scheme, PepperKeyID: h.active, PHC: phc}, nil
}

func (h *Hasher) Hash(ctx context.Context, value string) (Credential, error) {
	if err := ValidateNewPassword(value); err != nil {
		return Credential{}, err
	}
	if err := h.acquire(ctx); err != nil {
		return Credential{}, err
	}
	defer func() { <-h.slots }()
	return h.hash(ctx, value, h.params)
}

func number(value, prefix string, bits int) (uint64, error) {
	if !strings.HasPrefix(value, prefix) {
		return 0, ErrCredential
	}
	raw := strings.TrimPrefix(value, prefix)
	n, err := strconv.ParseUint(raw, 10, bits)
	if err != nil || strconv.FormatUint(n, 10) != raw {
		return 0, ErrCredential
	}
	return n, nil
}

func parse(phc string) (Params, []byte, []byte, error) {
	bad := func() (Params, []byte, []byte, error) { return Params{}, nil, nil, ErrCredential }
	if len(phc) > maxPHCBytes {
		return bad()
	}
	fields := strings.Split(phc, "$")
	if len(fields) != 6 || fields[0] != "" || fields[1] != "argon2id" || fields[2] != "v=19" {
		return bad()
	}
	limits := strings.Split(fields[3], ",")
	if len(limits) != 3 {
		return bad()
	}
	m, e1 := number(limits[0], "m=", 32)
	t, e2 := number(limits[1], "t=", 32)
	p, e3 := number(limits[2], "p=", 8)
	params := Params{MemoryKiB: uint32(m), Iterations: uint32(t), Parallelism: uint8(p)}
	if e1 != nil || e2 != nil || e3 != nil || !params.valid() {
		return bad()
	}
	salt, e1 := base64.RawStdEncoding.Strict().DecodeString(fields[4])
	sum, e2 := base64.RawStdEncoding.Strict().DecodeString(fields[5])
	if e1 != nil || e2 != nil || len(salt) < 16 || len(salt) > 32 || len(sum) != hashBytes {
		return bad()
	}
	return params, salt, sum, nil
}

// Verify accepts the stored, bounded cost, not caller-supplied work parameters.
// A wrong password never produces a replacement. Key upgrades preserve stronger
// stored memory/time costs. Cancellation cannot interrupt Argon2 itself: the
// concurrency slot remains held until the running computation finishes.
func (h *Hasher) Verify(ctx context.Context, value string, c Credential) (Verification, error) {
	if len(value) > MaxPasswordBytes || !utf8.ValidString(value) {
		return Verification{}, ErrPasswordInput
	}
	if c.Scheme != Scheme || !validKeyID(c.PepperKeyID) {
		return Verification{}, ErrCredential
	}
	p, salt, expected, err := parse(c.PHC)
	if err != nil {
		return Verification{}, err
	}
	key, ok := h.keys[c.PepperKeyID]
	if !ok {
		return Verification{}, ErrPepperUnavailable
	}
	if err = h.acquire(ctx); err != nil {
		return Verification{}, err
	}
	defer func() { <-h.slots }()
	actual := derive(value, key, salt, p)
	defer clear(actual)
	if err = ctx.Err(); err != nil {
		return Verification{}, err
	}
	if subtle.ConstantTimeCompare(actual, expected) != 1 {
		return Verification{}, nil
	}
	result := Verification{Match: true}
	if c.PepperKeyID != h.active || p.MemoryKiB < h.params.MemoryKiB || p.Iterations < h.params.Iterations {
		upgraded := h.params
		if p.MemoryKiB > upgraded.MemoryKiB {
			upgraded.MemoryKiB = p.MemoryKiB
		}
		if p.Iterations > upgraded.Iterations {
			upgraded.Iterations = p.Iterations
		}
		replacement, err := h.hash(ctx, value, upgraded)
		if err != nil {
			return Verification{}, err
		}
		result.Replacement = &replacement
	}
	return result, nil
}
