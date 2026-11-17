package password

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeyringGenerationRotationAndRestoredProbes(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old.json")
	next := filepath.Join(dir, "next.json")
	if err := GenerateKeyring(old, "old"); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(old)
	if err := GenerateKeyring(old, "replacement"); err == nil {
		t.Fatal("overwrote existing keyring")
	}
	after, _ := os.ReadFile(old)
	if !bytes.Equal(before, after) {
		t.Fatal("existing keyring changed")
	}
	if err := RotateKeyring(old, next, "current"); err != nil {
		t.Fatal(err)
	}
	oldRing, _ := LoadKeyring(old)
	ring, err := LoadKeyring(next)
	if err != nil {
		t.Fatal(err)
	}
	if ring.active != "current" || len(ring.keys) != 2 || !bytes.Equal(oldRing.keys["old"], ring.keys["old"]) {
		t.Fatal("rotation lost historical key")
	}
	if bytes.Equal(ring.keys["old"], ring.keys["current"]) {
		t.Fatal("keys were reused")
	}
	h, _ := oldRing.Hasher(testParams(), 1)
	c, err := h.Hash(context.Background(), examplePassword)
	if err != nil {
		t.Fatal(err)
	}
	h, _ = ring.Hasher(testParams(), 1)
	v, err := h.Verify(context.Background(), examplePassword, c)
	if err != nil || !v.Match || v.Replacement == nil || v.Replacement.PepperKeyID != "current" {
		t.Fatal("restored old credential cannot rotate")
	}
	probe := filepath.Join(dir, "probe.json")
	if err := WriteRecoveryProbe(context.Background(), next, probe); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "keyring.json")
	data, _ := os.ReadFile(next)
	if err := os.WriteFile(restored, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecoveryProbe(context.Background(), restored, probe); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecoveryProbe(context.Background(), old, probe); err == nil {
		t.Fatal("probe accepted missing historical/current key")
	}
	ring.keys["old"][0] ^= 1
	changed := filepath.Join(dir, "changed.json")
	if err := ring.save(changed); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRecoveryProbe(context.Background(), changed, probe); err == nil {
		t.Fatal("probe accepted incorrect historical key")
	}
	for _, value := range []any{ring} {
		encoded, _ := json.Marshal(value)
		if string(encoded) != "{}" {
			t.Fatal("keyring serialized")
		}
		for _, out := range []string{fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
			if !strings.Contains(out, "redacted") || strings.Contains(out, base64.StdEncoding.EncodeToString(ring.keys["current"])) {
				t.Fatal("keyring leaked")
			}
		}
	}
}

func TestKeyringRejectsUnsafeFilesAndSchema(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "ring.json")
	if err := GenerateKeyring(file, "current"); err != nil {
		t.Fatal(err)
	}
	valid, _ := os.ReadFile(file)
	for _, mode := range []os.FileMode{0644, 0640, 0620} {
		os.Chmod(file, mode)
		if _, err := LoadKeyring(file); !errors.Is(err, ErrKeyring) {
			t.Fatal("permissive file accepted")
		}
	}
	os.Chmod(file, 0600)
	link := filepath.Join(dir, "symlink")
	os.Symlink(file, link)
	fifo := filepath.Join(dir, "fifo")
	if err := unix.Mkfifo(fifo, 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{link, fifo, dir, filepath.Join(dir, "missing")} {
		if _, err := LoadKeyring(path); !errors.Is(err, ErrKeyring) {
			t.Fatal("unsafe path accepted")
		}
	}
	invalid := []string{
		`{}`, `null`, strings.Repeat("x", maxKeyringBytes+1), string(valid) + ` {}`,
		strings.Replace(string(valid), `"version":1`, `"version":2`, 1),
		strings.Replace(string(valid), `"version":1`, `"Version":1`, 1),
		strings.Replace(string(valid), `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(string(valid), `"version":1`, `"version":1,"extra":"hidden"`, 1),
		strings.Replace(string(valid), `"keys":{`, `"keys":{"current":"short",`, 1),
		strings.Replace(string(valid), `"active_key_id":"current"`, `"active_key_id":"missing"`, 1),
	}
	for _, data := range invalid {
		os.WriteFile(file, []byte(data), 0600)
		if _, err := LoadKeyring(file); !errors.Is(err, ErrKeyring) {
			t.Fatal("invalid document accepted")
		}
	}
	os.WriteFile(file, valid, 0600)
	if err := LoadConfigured("false", filepath.Join(dir, "missing")); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfigured("true", filepath.Join(dir, "missing")); !errors.Is(err, ErrKeyring) {
		t.Fatal("enabled configuration silently continued")
	}
	if err := LoadConfigured("true", file); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfigured("yes", file); !errors.Is(err, ErrConfiguration) {
		t.Fatal("invalid feature flag accepted")
	}
}
