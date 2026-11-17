package password

import (
	"context"
	"encoding/json"
)

// This public fixture is never a user's password. Probes belong only in protected
// files and encrypted snapshots, alongside (not instead of) the keyring.
const recoveryPassword = "devcourse-finder recovery probe v1"

type recoveryDocument struct {
	Version     int               `json:"version"`
	Scheme      string            `json:"scheme"`
	Credentials map[string]string `json:"credentials"`
}

func WriteRecoveryProbe(ctx context.Context, keyring, path string) error {
	k, err := LoadKeyring(keyring)
	if err != nil {
		return err
	}
	doc := recoveryDocument{Version: 1, Scheme: Scheme, Credentials: map[string]string{}}
	for id := range k.keys {
		h, err := New(DefaultParams(), id, k.keys, 1)
		if err != nil {
			return ErrKeyring
		}
		c, err := h.Hash(ctx, recoveryPassword)
		if err != nil {
			return err
		}
		doc.Credentials[id] = c.PHC
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return ErrCredential
	}
	defer clear(data)
	return writeSecret(path, data)
}

func VerifyRecoveryProbe(ctx context.Context, keyring, path string) error {
	k, err := LoadKeyring(keyring)
	if err != nil {
		return err
	}
	// Probes contain no keys, but remain private: reuse the protected-file reader
	// through a bounded descriptor (no symlinks/FIFOs and no permissive files).
	data, err := readPrivateFile(path, maxKeyringBytes)
	if err != nil {
		return err
	}
	defer clear(data)
	if !uniqueJSON(data) {
		return ErrCredential
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || len(fields) != 3 {
		return ErrCredential
	}
	for _, name := range []string{"version", "scheme", "credentials"} {
		if _, ok := fields[name]; !ok {
			return ErrCredential
		}
	}
	var doc recoveryDocument
	if json.Unmarshal(data, &doc) != nil || doc.Version != 1 || doc.Scheme != Scheme || len(doc.Credentials) != len(k.keys) {
		return ErrCredential
	}
	for id := range k.keys {
		phc, ok := doc.Credentials[id]
		if !ok {
			return ErrCredential
		}
		h, err := New(DefaultParams(), id, k.keys, 1)
		if err != nil {
			return ErrKeyring
		}
		result, err := h.Verify(ctx, recoveryPassword, Credential{Scheme: Scheme, PepperKeyID: id, PHC: phc})
		if err != nil {
			return err
		}
		if !result.Match {
			return ErrCredential
		}
	}
	return nil
}
