package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"devcourse-finder/auth/password"
)

// Invoked by scripts/test-user-recovery.sh across a real pg_dump/pg_restore.
// Only synthetic accounts and generated throwaway pepper keys are used.
func TestPostgresCredentialRecovery(t *testing.T) {
	stage := os.Getenv("AUTH_RECOVERY_STAGE")
	if stage == "" {
		t.Skip("run scripts/test-user-recovery.sh")
	}
	if stage != "seed" && stage != "verify" {
		t.Fatal("invalid recovery stage")
	}
	directory := os.Getenv("AUTH_RECOVERY_KEYS")
	if directory == "" {
		t.Fatal("private fixture directory required")
	}
	keyring := filepath.Join(directory, "password-keyring.json")
	probe := filepath.Join(directory, "password-recovery.json")
	ctx := context.Background()
	var db *DB
	if stage == "seed" {
		db = userTestDB(t)
	} else {
		// Reuse the URL safety check without migrating/truncating the restored data.
		raw := os.Getenv("TEST_DATABASE_URL")
		if !testDatabaseURL(raw) {
			t.Fatal("dedicated test database required")
		}
		var err error
		db, err = Open(ctx, raw)
		if err != nil {
			t.Fatal("restore DB unavailable")
		}
		t.Cleanup(db.Pool.Close)
	}
	if stage == "seed" {
		previous := filepath.Join(directory, "old.json")
		if password.GenerateKeyring(previous, "old") != nil || password.RotateKeyring(previous, keyring, "current") != nil || password.WriteRecoveryProbe(ctx, keyring, probe) != nil {
			t.Fatal("keyring fixture failed")
		}
		for _, fixture := range []struct{ file, email, value string }{{previous, "old-recovery@example.test", accountTestPassword}, {keyring, "current-recovery@example.test", changedTestPassword}} {
			ring, err := password.LoadKeyring(fixture.file)
			if err != nil {
				t.Fatal(err)
			}
			h, _ := ring.Hasher(password.DefaultParams(), 1)
			c, err := h.Hash(ctx, fixture.value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Users().Create(ctx, fixture.email, c); err != nil {
				t.Fatal("fixture create failed")
			}
		}
	}
	if err := password.VerifyRecoveryProbe(ctx, keyring, probe); err != nil {
		t.Fatal("restored keyring probe failed")
	}
	ring, err := password.LoadKeyring(keyring)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := ring.Hasher(password.DefaultParams(), 1)
	for email, value := range map[string]string{"old-recovery@example.test": accountTestPassword, "current-recovery@example.test": changedTestPassword} {
		c, err := db.Users().ByEmail(ctx, email)
		if err != nil {
			t.Fatal("restored account missing")
		}
		v, err := h.Verify(ctx, value, c.Credential)
		if err != nil || !v.Match {
			t.Fatal("restored credential failed")
		}
		if c.Credential.PepperKeyID == "old" {
			if v.Replacement == nil {
				t.Fatal("historical credential did not upgrade")
			}
			if stage == "verify" {
				ok, err := db.Users().Upgrade(ctx, c, *v.Replacement)
				if err != nil || !ok {
					t.Fatal("restored historical upgrade failed")
				}
			}
		}
	}
}
