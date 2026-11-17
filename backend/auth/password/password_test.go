package password

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

const examplePassword = "a long example passphrase"

func testHasher(t testing.TB, active string, keys map[string][]byte, params Params) *Hasher {
	t.Helper()
	h, err := New(params, active, keys, 1)
	if err != nil {
		t.Fatal(err)
	}
	return h
}
func testParams() Params          { return Params{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1} }
func testKeys() map[string][]byte { return map[string][]byte{"current": bytes.Repeat([]byte{7}, 32)} }

func TestRoundTripSaltAndExactPasswordBytes(t *testing.T) {
	h := testHasher(t, "current", testKeys(), testParams())
	a, err := h.Hash(context.Background(), examplePassword)
	if err != nil {
		t.Fatal(err)
	}
	b, err := h.Hash(context.Background(), examplePassword)
	if err != nil {
		t.Fatal(err)
	}
	if a.PHC == b.PHC {
		t.Fatal("passwords must receive independent random salts")
	}
	for _, value := range []string{examplePassword, examplePassword + " ", "wrong password", ""} {
		result, err := h.Verify(context.Background(), value, a)
		if err != nil {
			t.Fatal(err)
		}
		if result.Match != (value == examplePassword) || result.Replacement != nil {
			t.Fatal("unexpected verification result")
		}
	}
}

func TestPepperRotationAndCostUpgrade(t *testing.T) {
	keys := testKeys()
	keys["old"] = bytes.Repeat([]byte{9}, 32)
	oldParams := testParams()
	oldParams.MemoryKiB = 20 * 1024
	old := testHasher(t, "old", keys, oldParams)
	c, err := old.Hash(context.Background(), examplePassword)
	if err != nil {
		t.Fatal(err)
	}
	target := testParams()
	target.Iterations = 3
	current := testHasher(t, "current", keys, target)
	wrong, err := current.Verify(context.Background(), "incorrect", c)
	if err != nil || wrong.Match || wrong.Replacement != nil {
		t.Fatal("wrong password cannot upgrade credentials")
	}
	result, err := current.Verify(context.Background(), examplePassword, c)
	if err != nil || !result.Match || result.Replacement == nil {
		t.Fatal("valid old credentials should upgrade")
	}
	replacement := *result.Replacement
	if replacement.PepperKeyID != "current" || replacement.PHC == c.PHC {
		t.Fatal("key rotation did not create a fresh credential")
	}
	p, _, _, err := parse(replacement.PHC)
	if err != nil || p.MemoryKiB != oldParams.MemoryKiB || p.Iterations != target.Iterations {
		t.Fatal("upgrade lowered an existing cost")
	}
	result, err = current.Verify(context.Background(), examplePassword, replacement)
	if err != nil || !result.Match || result.Replacement != nil {
		t.Fatal("upgraded credential should remain stable")
	}
	missing := testHasher(t, "current", testKeys(), testParams())
	if _, err = missing.Verify(context.Background(), examplePassword, c); !errors.Is(err, ErrPepperUnavailable) {
		t.Fatal("missing old key must fail closed")
	}
	wrongKeys := testKeys()
	wrongKeys["old"] = bytes.Repeat([]byte{8}, 32)
	wrongHasher := testHasher(t, "current", wrongKeys, testParams())
	result, err = wrongHasher.Verify(context.Background(), examplePassword, c)
	if err != nil || result.Match {
		t.Fatal("a different pepper must not verify the credential")
	}
}

func TestCredentialParserRejectsUnsafeCostsBeforeCapacityCheck(t *testing.T) {
	h := testHasher(t, "current", testKeys(), testParams())
	c, err := h.Hash(context.Background(), examplePassword)
	if err != nil {
		t.Fatal(err)
	}
	h.slots <- struct{}{}
	defer func() { <-h.slots }()
	invalid := []string{
		strings.Repeat("x", maxPHCBytes+1), strings.Replace(c.PHC, "argon2id", "argon2i", 1),
		strings.Replace(c.PHC, "v=19", "v=16", 1), strings.Replace(c.PHC, "m=19456", "m=4294967296", 1),
		strings.Replace(c.PHC, "m=19456", "m=1", 1), strings.Replace(c.PHC, "t=2", "t=4294967295", 1),
		strings.Replace(c.PHC, "t=2", "t=02", 1), strings.Replace(c.PHC, "p=1", "p=0", 1),
		strings.Replace(c.PHC, "p=1", "p=255", 1), c.PHC + "$extra",
	}
	parts := strings.Split(c.PHC, "$")
	badSalt := append([]string(nil), parts...)
	badSalt[4] = "bad"
	invalid = append(invalid, strings.Join(badSalt, "$"))
	badSum := append([]string(nil), parts...)
	badSum[5] = "!"
	invalid = append(invalid, strings.Join(badSum, "$"))
	for _, phc := range invalid {
		modified := c
		modified.PHC = phc
		if _, err = h.Verify(context.Background(), examplePassword, modified); !errors.Is(err, ErrCredential) {
			t.Fatal("unsafe credential accepted or hashing attempted")
		}
	}
}

func TestCapacityCancellationAndConcurrentUse(t *testing.T) {
	h := testHasher(t, "current", testKeys(), testParams())
	c, err := h.Hash(context.Background(), examplePassword)
	if err != nil {
		t.Fatal(err)
	}
	h.slots <- struct{}{}
	if _, err = h.Hash(context.Background(), examplePassword); !errors.Is(err, ErrBusy) {
		t.Fatal("hashing should fail fast when full")
	}
	if _, err = h.Verify(context.Background(), examplePassword, c); !errors.Is(err, ErrBusy) {
		t.Fatal("verification should share the same gate")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = h.Hash(ctx, examplePassword); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled work was accepted")
	}
	<-h.slots
	var wg sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.Hash(context.Background(), examplePassword)
			if err != nil && !errors.Is(err, ErrBusy) {
				failures <- err
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
	if len(h.slots) != 0 {
		t.Fatal("capacity leaked")
	}
}

func TestInputPolicyAndConfiguration(t *testing.T) {
	for _, value := range []string{"short", strings.Repeat("x", 1025), string([]byte{0xff})} {
		if !errors.Is(ValidateNewPassword(value), ErrPasswordInput) {
			t.Fatal("invalid input accepted")
		}
	}
	for _, value := range []string{strings.Repeat("я", 15), strings.Repeat("x", 1024), "  a long passphrase  "} {
		if err := ValidateNewPassword(value); err != nil {
			t.Fatal("valid passphrase rejected")
		}
	}
	for _, p := range []Params{{}, {MemoryKiB: 128*1024 + 1, Iterations: 3, Parallelism: 1}, {MemoryKiB: 64 * 1024, Iterations: 0, Parallelism: 1}} {
		if _, err := New(p, "current", testKeys(), 1); !errors.Is(err, ErrConfiguration) {
			t.Fatal("unsafe configuration accepted")
		}
	}
	if _, err := New(testParams(), "missing", testKeys(), 1); !errors.Is(err, ErrConfiguration) {
		t.Fatal("active key is required")
	}
	if _, err := New(testParams(), "current", testKeys(), 3); !errors.Is(err, ErrConfiguration) {
		t.Fatal("unbounded concurrency accepted")
	}
}

func TestKeysAreCopiedAndCredentialsRedacted(t *testing.T) {
	keys := testKeys()
	original := append([]byte(nil), keys["current"]...)
	h := testHasher(t, "current", keys, testParams())
	clear(keys["current"])
	delete(keys, "current")
	c, err := h.Hash(context.Background(), examplePassword)
	if err != nil {
		t.Fatal(err)
	}
	other := testHasher(t, "current", map[string][]byte{"current": original}, testParams())
	result, err := other.Verify(context.Background(), examplePassword, c)
	if err != nil || !result.Match {
		t.Fatal("caller mutated the stored key")
	}
	encoded, err := json.Marshal(c)
	if err != nil || string(encoded) != "{}" {
		t.Fatal("credential exposed through JSON")
	}
	for _, text := range []string{fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c), fmt.Sprintf("%+v", h), fmt.Sprintf("%#v", h)} {
		if strings.Contains(text, c.PHC) || strings.Contains(text, examplePassword) || !strings.Contains(text, "redacted") {
			t.Fatal("credential material exposed by formatting")
		}
	}
}

func BenchmarkHashDefault(b *testing.B) {
	h := testHasher(b, "current", testKeys(), DefaultParams())
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := h.Hash(context.Background(), examplePassword); err != nil {
			b.Fatal(err)
		}
	}
}
func BenchmarkVerifyDefault(b *testing.B) {
	h := testHasher(b, "current", testKeys(), DefaultParams())
	c, err := h.Hash(context.Background(), examplePassword)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		result, err := h.Verify(context.Background(), examplePassword, c)
		if err != nil || !result.Match {
			b.Fatal("verification failed")
		}
	}
}
