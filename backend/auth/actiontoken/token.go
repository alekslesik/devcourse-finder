// Package actiontoken provides bearer secrets for explicit account actions.
package actiontoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"time"
)

type Purpose string

const (
	ConfirmEmail  Purpose = "confirm_email"
	ResetPassword Purpose = "reset_password"
)

var ErrInvalid = errors.New("invalid account action token")

func (p Purpose) Valid() bool { return p == ConfirmEmail || p == ResetPassword }
func (p Purpose) TTL() time.Duration {
	if p == ConfirmEmail {
		return 24 * time.Hour
	}
	if p == ResetPassword {
		return 15 * time.Minute
	}
	return 0
}

type Secret struct{ value [32]byte }

func (Secret) String() string               { return "actiontoken.Secret(redacted)" }
func (s Secret) GoString() string           { return s.String() }
func (Secret) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }
func Generate() (Secret, error) {
	var s Secret
	if _, err := rand.Read(s.value[:]); err != nil {
		return Secret{}, ErrInvalid
	}
	return s, nil
}
func Parse(raw string) (Secret, error) {
	var s Secret
	if len(raw) != 46 || !strings.HasPrefix(raw, "v1.") {
		return s, ErrInvalid
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(raw[3:])
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw[3:] {
		return s, ErrInvalid
	}
	copy(s.value[:], decoded)
	clear(decoded)
	return s, nil
}

// Encoded is an explicit secret export for email links, never logging or JSON.
func (s Secret) Encoded() string { return "v1." + base64.RawURLEncoding.EncodeToString(s.value[:]) }
func (s Secret) Digest(p Purpose) ([32]byte, error) {
	if !p.Valid() {
		return [32]byte{}, ErrInvalid
	}
	h := sha256.New()
	h.Write([]byte("devcourse-finder/account-action/v1\x00"))
	h.Write([]byte(p))
	h.Write([]byte{0})
	h.Write(s.value[:])
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}

type Origin struct{ value string }

func NewOrigin(raw string) (Origin, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || (u.Port() != "" && u.Port() != "443") {
		return Origin{}, ErrInvalid
	}
	host := u.Hostname()
	if host != strings.ToLower(host) || !strings.Contains(host, ".") {
		return Origin{}, ErrInvalid
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return Origin{}, ErrInvalid
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return Origin{}, ErrInvalid
			}
		}
	}
	if len(host) > 253 {
		return Origin{}, ErrInvalid
	}
	return Origin{value: "https://" + host}, nil
}

// Link puts the token only in a fragment: an ordinary HTTP GET/prefetch and
// Referer never carry it. The future page must scrub it and require explicit POST.
func (o Origin) Link(p Purpose, s Secret) (string, error) {
	if o.value == "" || !p.Valid() {
		return "", ErrInvalid
	}
	route := "/account/confirm-email"
	if p == ResetPassword {
		route = "/account/reset-password"
	}
	return o.value + route + "#token=" + url.QueryEscape(s.Encoded()), nil
}
