package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"devcourse-finder/auth/password"
)

func TestNormalizeAccountEmail(t *testing.T) {
	for raw, want := range map[string]string{" Alice.Tag+Topic@Example.TEST ": "alice.tag+topic@example.test", "user@xn--e1afmkfd.xn--p1ai": "user@xn--e1afmkfd.xn--p1ai"} {
		got, err := NormalizeEmail(raw)
		if err != nil || got != want {
			t.Fatal("unexpected email normalization")
		}
	}
	for _, raw := range []string{"", "a@localhost", "Name <a@example.test>", "\"a\"@example.test", "a..b@example.test", ".a@example.test", "a.@example.test", "a@-example.test", "a@exam_ple.test", "a@пример.рф", "K@example.test", "İ@example.test", strings.Repeat("a", 65) + "@example.test", "a@" + strings.Repeat("b", 64) + ".test", "a\n@example.test", strings.Repeat("x", 1025)} {
		if _, err := NormalizeEmail(raw); !errors.Is(err, ErrUserInput) {
			t.Fatal("unsafe email accepted")
		}
	}
}
func userTestDB(t *testing.T) *DB {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL required for user storage integration")
	}
	if !testDatabaseURL(raw) {
		t.Fatal("dedicated database ending _test required")
	}
	d, err := Open(context.Background(), raw)
	if err != nil {
		t.Fatal("test DB unavailable")
	}
	t.Cleanup(d.Pool.Close)
	for range 2 {
		if d.Migrate(context.Background()) != nil {
			t.Fatal("migration failed")
		}
	}
	if _, err = d.Pool.Exec(context.Background(), "TRUNCATE auth_users CASCADE"); err != nil {
		t.Fatal("test cleanup failed")
	}
	return d
}
func userTestHashers(t *testing.T) (*password.Hasher, *password.Hasher) {
	t.Helper()
	keys := map[string][]byte{"old": bytes.Repeat([]byte{1}, 32), "current": bytes.Repeat([]byte{2}, 32)}
	p := password.Params{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1}
	old, err := password.New(p, "old", keys, 1)
	if err != nil {
		t.Fatal(err)
	}
	p.Iterations = 3
	next, err := password.New(p, "current", keys, 1)
	if err != nil {
		t.Fatal(err)
	}
	return old, next
}

const accountTestPassword = "a long original test passphrase"
const changedTestPassword = "a different new test passphrase"

func TestPostgresPrivateUserStorageAndRotation(t *testing.T) {
	db := userTestDB(t)
	repo := db.Users()
	ctx := context.Background()
	old, current := userTestHashers(t)
	c, err := old.Hash(ctx, accountTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	user, err := repo.Create(ctx, " Alice.Tag+Topic@Example.TEST ", c)
	if err != nil {
		t.Fatal(err)
	}
	if !userIDPattern.MatchString(user.ID) || user.Email != "alice.tag+topic@example.test" || user.EmailVerifiedAt != nil || user.CreatedAt.IsZero() {
		t.Fatal("incorrect new user state")
	}
	if _, err = repo.Create(ctx, "ALICE.TAG+TOPIC@example.test", c); !errors.Is(err, ErrEmailConflict) {
		t.Fatal("case collision not rejected")
	}
	observed, err := repo.ByEmail(ctx, "ALICE.TAG+TOPIC@example.test")
	if err != nil {
		t.Fatal(err)
	}
	if observed.User.ID != user.ID || observed.Revision != 1 || observed.PasswordVersion != 1 {
		t.Fatal("incorrect credential state")
	}
	wrong, err := current.Verify(ctx, "incorrect", observed.Credential)
	if err != nil || wrong.Match || wrong.Replacement != nil {
		t.Fatal("wrong password verified")
	}
	missing, _ := password.New(password.DefaultParams(), "current", map[string][]byte{"current": bytes.Repeat([]byte{2}, 32)}, 1)
	if _, err = missing.Verify(ctx, accountTestPassword, observed.Credential); !errors.Is(err, password.ErrPepperUnavailable) {
		t.Fatal("missing historical pepper accepted")
	}
	verified, err := current.Verify(ctx, accountTestPassword, observed.Credential)
	if err != nil || !verified.Match || verified.Replacement == nil {
		t.Fatal("old credential failed verification/rotation")
	}
	ok, err := repo.Upgrade(ctx, observed, *verified.Replacement)
	if err != nil || !ok {
		t.Fatal("credential upgrade failed")
	}
	updated, err := repo.ByID(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Revision != 2 || updated.PasswordVersion != 1 || !updated.PasswordChangedAt.Equal(observed.PasswordChangedAt) || updated.Credential.PepperKeyID != "current" {
		t.Fatal("upgrade changed password age/version")
	}
	if ok, err = repo.Upgrade(ctx, observed, *verified.Replacement); err != nil || ok {
		t.Fatal("stale upgrade accepted")
	}
	different, err := current.Hash(ctx, changedTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	forged := updated
	forged.Credential = different
	if ok, err = repo.Upgrade(ctx, forged, different); err != nil || ok {
		t.Fatal("CAS ignored original credential")
	}
	for _, v := range []any{user, observed, updated} {
		data, _ := json.Marshal(v)
		if string(data) != "{}" {
			t.Fatal("private record serialized")
		}
		for _, text := range []string{fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v)} {
			if !strings.Contains(text, "redacted") || strings.Contains(text, user.Email) || strings.Contains(text, c.PHC) {
				t.Fatal("private record formatted")
			}
		}
	}
	invalid := c
	invalid.PHC = "PRIVATE_SENTINEL"
	if _, err = repo.Create(ctx, "invalid@example.test", invalid); !errors.Is(err, ErrUserInput) || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
		t.Fatal("invalid credential accepted/leaked")
	}
	var users, creds int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM auth_users").Scan(&users)
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM auth_credentials").Scan(&creds)
	if users != 1 || creds != 1 {
		t.Fatal("create/conflict left partial rows")
	}
}

func TestPostgresConcurrentResetAndUpgrade(t *testing.T) {
	db := userTestDB(t)
	repo := db.Users()
	ctx := context.Background()
	old, current := userTestHashers(t)
	c, _ := old.Hash(ctx, accountTestPassword)
	u, err := repo.Create(ctx, "race@example.test", c)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := repo.ByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	v, err := current.Verify(ctx, accountTestPassword, observed.Credential)
	if err != nil || v.Replacement == nil {
		t.Fatal("rotation fixture failed")
	}
	reset, err := current.Hash(ctx, changedTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	var upgradeOK, resetOK bool
	var upgradeErr, resetErr error
	wg.Add(2)
	go func() { defer wg.Done(); <-start; upgradeOK, upgradeErr = repo.Upgrade(ctx, observed, *v.Replacement) }()
	go func() { defer wg.Done(); <-start; resetOK, resetErr = repo.ChangePassword(ctx, observed, reset) }()
	close(start)
	wg.Wait()
	if upgradeErr != nil || resetErr != nil || upgradeOK == resetOK {
		t.Fatal("exactly one concurrent CAS must win")
	}
	if !resetOK {
		latest, err := repo.ByID(ctx, u.ID)
		if err != nil {
			t.Fatal(err)
		}
		resetOK, err = repo.ChangePassword(ctx, latest, reset)
		if err != nil || !resetOK {
			t.Fatal("reset retry against current revision failed")
		}
	}
	if ok, err := repo.Upgrade(ctx, observed, *v.Replacement); err != nil || ok {
		t.Fatal("stale upgrade overwrote reset")
	}
	final, err := repo.ByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.PasswordVersion != 2 || !final.PasswordChangedAt.After(observed.PasswordChangedAt) {
		t.Fatal("reset did not advance password version/time")
	}
	for input, want := range map[string]bool{accountTestPassword: false, changedTestPassword: true} {
		result, err := current.Verify(ctx, input, final.Credential)
		if err != nil || result.Match != want {
			t.Fatal("reset credential lost")
		}
	}
}

func TestPostgresUserTransactionRollbackAndConstraints(t *testing.T) {
	db := userTestDB(t)
	repo := db.Users()
	ctx := context.Background()
	old, _ := userTestHashers(t)
	c, _ := old.Hash(ctx, accountTestPassword)
	rollback := errors.New("rollback fixture")
	err := db.WithUsers(ctx, func(tx *UserStore) error {
		if _, err := tx.Create(ctx, "rollback@example.test", c); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("transaction error lost")
	}
	if _, err = repo.ByEmail(ctx, "rollback@example.test"); !errors.Is(err, ErrUserNotFound) {
		t.Fatal("nested create escaped rollback")
	}
	user, err := repo.Create(ctx, "keep@example.test", c)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, _ := repo.ByID(ctx, user.ID)
	next, _ := old.Hash(ctx, changedTestPassword)
	err = db.WithUsers(ctx, func(tx *UserStore) error {
		ok, err := tx.ChangePassword(ctx, snapshot, next)
		if err != nil || !ok {
			return ErrUserStorage
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("reset rollback failed")
	}
	after, _ := repo.ByID(ctx, user.ID)
	if after.Revision != snapshot.Revision || after.Credential.PHC != snapshot.Credential.PHC {
		t.Fatal("reset escaped rollback")
	}
	for _, email := range []string{"UPPER@example.test", "a..b@example.test", "display <a@example.test>"} {
		id, _ := newUserID()
		if _, err = db.Pool.Exec(ctx, "INSERT INTO auth_users(id,email) VALUES($1,$2)", id, email); err == nil {
			t.Fatal("database accepted noncanonical email")
		}
	}
	if _, err = db.Pool.Exec(ctx, "UPDATE auth_credentials SET phc='' WHERE user_id=$1", user.ID); err == nil {
		t.Fatal("empty credential accepted")
	}
	if _, err = db.Pool.Exec(ctx, "UPDATE auth_credentials SET revision=0 WHERE user_id=$1", user.ID); err == nil {
		t.Fatal("invalid revision accepted")
	}
	if _, err = db.Pool.Exec(ctx, "DELETE FROM auth_users WHERE id=$1", user.ID); err != nil {
		t.Fatal("delete failed")
	}
	if _, err = repo.ByID(ctx, user.ID); !errors.Is(err, ErrUserNotFound) {
		t.Fatal("credential did not cascade")
	}
}

func testDatabaseURL(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && strings.HasSuffix(parsed.Path, "_test")
}
