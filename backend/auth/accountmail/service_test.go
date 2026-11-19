package accountmail

import (
	"bytes"
	"context"
	"crypto/rand"
	"devcourse-finder/auth/actiontoken"
	"devcourse-finder/auth/password"
	"devcourse-finder/store"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/url"
	"os"
	"strings"
	"testing"
)

type fakeSender struct {
	messages []Message
	fail     bool
}

func (f *fakeSender) Send(_ context.Context, m Message) error {
	f.messages = append(f.messages, m)
	if f.fail {
		return errors.New("PRIVATE_SMTP_DETAIL")
	}
	return nil
}
func accountMailDB(t *testing.T) *store.DB {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("PostgreSQL integration required")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("dedicated test DB required")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal("test database unavailable")
	}
	var random [8]byte
	rand.Read(random[:])
	name := "mail_test_" + hex.EncodeToString(random[:])
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Pool.Exec(ctx, "CREATE SCHEMA "+identifier); err != nil {
		t.Fatal("fixture schema failed")
	}
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	db, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal("fixture open failed")
	}
	t.Cleanup(func() {
		db.Pool.Close()
		admin.Pool.Exec(ctx, "DROP SCHEMA "+identifier+" CASCADE")
		admin.Pool.Close()
	})
	if db.Migrate(ctx) != nil {
		t.Fatal("fixture migration failed")
	}
	return db
}
func rawLink(t *testing.T, m Message) string {
	t.Helper()
	u, err := url.Parse(m.Link)
	if err != nil {
		t.Fatal("invalid link")
	}
	fragment, err := url.ParseQuery(u.Fragment)
	if err != nil {
		t.Fatal("invalid fragment")
	}
	return fragment.Get("token")
}
func commonFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("../password/blocklist/known-common.txt")
	if err != nil {
		t.Fatal("public fixture unavailable")
	}
	return strings.TrimSpace(string(data))
}
func TestPostgresAccountMailRetryConfirmAndReset(t *testing.T) {
	db := accountMailDB(t)
	ctx := context.Background()
	repo := db.Users()
	h, err := password.New(password.Params{MemoryKiB: 19 * 1024, Iterations: 2, Parallelism: 1}, "current", map[string][]byte{"current": bytes.Repeat([]byte{3}, 32)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	c, _ := h.Hash(ctx, "an original synthetic passphrase")
	user, err := repo.Create(ctx, "mail@example.test", c)
	if err != nil {
		t.Fatal(err)
	}
	sender := &fakeSender{fail: true}
	origin, _ := actiontoken.NewOrigin("https://example.test")
	service, err := New(repo, sender, origin, h)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Request(ctx, "missing@example.test", actiontoken.ConfirmEmail); err != nil {
		t.Fatal("unknown account response differs")
	}
	if sent, err := service.SendOne(ctx); err != nil || sent {
		t.Fatal("unknown account queued mail")
	}
	if err = service.Request(ctx, user.Email, actiontoken.ConfirmEmail); err != nil {
		t.Fatal(err)
	}
	if attempted, err := service.SendOne(ctx); !attempted || err != ErrTransport || strings.Contains(err.Error(), "PRIVATE_SMTP") {
		t.Fatal("failure leaked or wasn't queued for retry")
	}
	old := rawLink(t, sender.messages[0])
	if err = service.Confirm(ctx, old); err != store.ErrActionToken {
		t.Fatal("failed attempt token accepted")
	}
	db.Pool.Exec(ctx, "UPDATE auth_mail_jobs SET available_at=clock_timestamp()-interval '1 second'")
	sender.fail = false
	if attempted, err := service.SendOne(ctx); err != nil || !attempted {
		t.Fatal("retry failed")
	}
	confirmation := rawLink(t, sender.messages[1])
	if old == confirmation {
		t.Fatal("retry persisted/reused raw token")
	}
	if err = service.Confirm(ctx, confirmation); err != nil {
		t.Fatal(err)
	}
	if err = service.Request(ctx, user.Email, actiontoken.ResetPassword); err != nil {
		t.Fatal(err)
	}
	if attempted, err := service.SendOne(ctx); err != nil || !attempted {
		t.Fatal("reset email failed")
	}
	reset := rawLink(t, sender.messages[2])
	if err = service.Reset(ctx, reset, commonFixture(t)); err != password.ErrCommonPassword {
		t.Fatal("known common password accepted")
	}
	if _, err = repo.TokenSubject(ctx, reset, actiontoken.ResetPassword); err != nil {
		t.Fatal("policy failure consumed token")
	}
	if err = service.Reset(ctx, reset, "a different synthetic new passphrase"); err != nil {
		t.Fatal(err)
	}
	after, err := repo.ByID(ctx, user.ID)
	if err != nil || after.PasswordVersion != 2 {
		t.Fatal("reset version not advanced")
	}
	v, err := h.Verify(ctx, "a different synthetic new passphrase", after.Credential)
	if err != nil || !v.Match {
		t.Fatal("new password not persisted")
	}
	if err = service.Reset(ctx, reset, "one more synthetic new passphrase"); err != store.ErrActionToken {
		t.Fatal("token replay accepted")
	}
}
