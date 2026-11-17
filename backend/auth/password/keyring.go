package password

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

const maxKeyringBytes = 16384

var ErrKeyring = errors.New("invalid or unavailable password keyring")

// Keyring holds immutable secrets. Never serialize its contents to diagnostics.
type Keyring struct {
	active string
	keys   map[string][]byte
}

func (*Keyring) String() string               { return "password.Keyring(redacted)" }
func (k *Keyring) GoString() string           { return k.String() }
func (*Keyring) MarshalJSON() ([]byte, error) { return []byte(`{}`), nil }
func (k *Keyring) Hasher(p Params, concurrent int) (*Hasher, error) {
	return New(p, k.active, k.keys, concurrent)
}

type keyringDocument struct {
	Version int               `json:"version"`
	Active  string            `json:"active_key_id"`
	Keys    map[string]string `json:"keys"`
}

// uniqueJSON rejects duplicate fields at every level, including credential key IDs.
func uniqueJSON(data []byte) bool {
	d := json.NewDecoder(bytes.NewReader(data))
	var value func(int) bool
	value = func(depth int) bool {
		if depth > 8 {
			return false
		}
		t, err := d.Token()
		if err != nil {
			return false
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return true
		}
		if delim != '{' && delim != '[' {
			return false
		}
		seen := map[string]bool{}
		for d.More() {
			if delim == '{' {
				t, err = d.Token()
				name, ok := t.(string)
				if err != nil || !ok || seen[name] {
					return false
				}
				seen[name] = true
			}
			if !value(depth + 1) {
				return false
			}
		}
		end, err := d.Token()
		if err != nil {
			return false
		}
		return end == json.Delim(map[json.Delim]rune{'{': '}', '[': ']'}[delim])
	}
	if !value(0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

func decodeKeyring(data []byte) (*Keyring, error) {
	if len(data) > maxKeyringBytes || !uniqueJSON(data) {
		return nil, ErrKeyring
	}
	// Exact field names (encoding/json otherwise accepts case-insensitive names).
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 3 {
		return nil, ErrKeyring
	}
	for _, name := range []string{"version", "active_key_id", "keys"} {
		if _, ok := fields[name]; !ok {
			return nil, ErrKeyring
		}
	}
	var doc keyringDocument
	if json.Unmarshal(data, &doc) != nil || doc.Version != 1 || !validKeyID(doc.Active) || len(doc.Keys) < 1 || len(doc.Keys) > 8 {
		return nil, ErrKeyring
	}
	keys := make(map[string][]byte, len(doc.Keys))
	for id, encoded := range doc.Keys {
		key, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || !validKeyID(id) || len(key) < 32 || len(key) > 64 || base64.StdEncoding.EncodeToString(key) != encoded {
			return nil, ErrKeyring
		}
		keys[id] = key
	}
	if _, ok := keys[doc.Active]; !ok {
		return nil, ErrKeyring
	}
	return &Keyring{active: doc.Active, keys: keys}, nil
}

func readPrivateFile(path string, limit int64) ([]byte, error) {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return nil, ErrKeyring
	}
	f := os.NewFile(uintptr(fd), "password-keyring")
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, ErrKeyring
	}
	var st unix.Stat_t
	if unix.Fstat(fd, &st) != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > limit || (st.Uid != 0 && st.Uid != uint32(os.Geteuid())) {
		return nil, ErrKeyring
	}
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, ErrKeyring
	}
	return data, nil
}

// LoadKeyring accepts only a bounded regular file owned by root or this user,
// with no group/other permissions. The final path must not be a symlink.
// Callers must place it in a trusted directory outside source/build contexts.
func LoadKeyring(path string) (*Keyring, error) {
	data, err := readPrivateFile(path, maxKeyringBytes)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	return decodeKeyring(data)
}

// writeSecret publishes a fully written, fsynced file without replacing a path.
func writeSecret(path string, data []byte) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	var st unix.Stat_t
	if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 || unix.Stat(dir, &st) != nil || (st.Uid != 0 && st.Uid != uint32(os.Geteuid())) {
		return ErrKeyring
	}
	f, err := os.CreateTemp(dir, ".password-secret-*")
	if err != nil {
		return ErrKeyring
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if f.Chmod(0600) != nil {
		return ErrKeyring
	}
	if _, err = f.Write(data); err != nil {
		return ErrKeyring
	}
	if f.Sync() != nil || f.Close() != nil {
		return ErrKeyring
	}
	if os.Link(f.Name(), path) != nil {
		return ErrKeyring
	}
	d, err := os.Open(dir)
	if err != nil {
		return ErrKeyring
	}
	defer d.Close()
	if d.Sync() != nil {
		return ErrKeyring
	}
	return nil
}

func (k *Keyring) save(path string) error {
	doc := keyringDocument{Version: 1, Active: k.active, Keys: map[string]string{}}
	for id, key := range k.keys {
		doc.Keys[id] = base64.StdEncoding.EncodeToString(key)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return ErrKeyring
	}
	defer clear(data)
	return writeSecret(path, data)
}

// GenerateKeyring is an administrative operation, never an application startup fallback.
func GenerateKeyring(path, id string) error {
	if !validKeyID(id) {
		return ErrKeyring
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return ErrEntropy
	}
	defer clear(key)
	return (&Keyring{active: id, keys: map[string][]byte{id: key}}).save(path)
}

// RotateKeyring writes a NEW candidate file and retains every historical key.
// Activation must be serialized with deployment/backup and checked separately.
func RotateKeyring(source, dest, id string) error {
	k, err := LoadKeyring(source)
	if err != nil {
		return err
	}
	if !validKeyID(id) || len(k.keys) >= 8 {
		return ErrKeyring
	}
	if _, ok := k.keys[id]; ok {
		return ErrKeyring
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return ErrEntropy
	}
	defer clear(key)
	k.keys[id] = key
	k.active = id
	return k.save(dest)
}

// LoadConfigured is a startup preflight. Enabling password storage does not
// create public authentication routes. Disabled storage needs no secret file.
func LoadConfigured(enabled, path string) error {
	switch enabled {
	case "", "false":
		return nil
	case "true":
		k, err := LoadKeyring(path)
		if err != nil {
			return err
		}
		_, err = k.Hasher(DefaultParams(), 1)
		return err
	default:
		return ErrConfiguration
	}
}
