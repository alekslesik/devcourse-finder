package store

import (
	"context"
	"devcourse-finder/auth/actiontoken"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func actionFixture(t *testing.T) (*DB, *UserStore, UserCredential) {
	t.Helper()
	db := userTestDB(t)
	old, _ := userTestHashers(t)
	c, err := old.Hash(context.Background(), accountTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	u, err := db.Users().Create(context.Background(), "action@example.test", c)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := db.Users().ByID(context.Background(), u.ID)
	if err != nil {
		t.Fatal(err)
	}
	return db, db.Users(), snapshot
}
func requestDelivery(t *testing.T, repo *UserStore, email string, p actiontoken.Purpose) *Delivery {
	t.Helper()
	ctx := context.Background()
	if err := repo.RequestAction(ctx, email, p); err != nil {
		t.Fatal(err)
	}
	d, err := repo.ClaimDelivery(ctx)
	if err != nil || d == nil {
		t.Fatal("delivery unavailable", err)
	}
	return d
}
func TestPostgresAccountActionsAtomicAndVersionBound(t *testing.T) {
	db, repo, original := actionFixture(t)
	ctx := context.Background()
	for _, email := range []string{"unknown@example.test", "not-an-email"} {
		if repo.RequestAction(ctx, email, actiontoken.ConfirmEmail) != nil {
			t.Fatal("unknown account response differs")
		}
	}
	if repo.RequestAction(ctx, original.User.Email, actiontoken.ResetPassword) != nil {
		t.Fatal("ineligible request differs")
	}
	d := requestDelivery(t, repo, original.User.Email, actiontoken.ConfirmEmail)
	if err := repo.RequestAction(ctx, original.User.Email, actiontoken.ConfirmEmail); err != nil {
		t.Fatal(err)
	}
	if extra, err := repo.ClaimDelivery(ctx); err != nil || extra != nil {
		t.Fatal("duplicate request bypassed limits")
	}
	var digest []byte
	db.Pool.QueryRow(ctx, "SELECT digest FROM auth_action_tokens").Scan(&digest)
	expected, _ := d.Secret.Digest(d.Purpose)
	if string(digest) != string(expected[:]) {
		t.Fatal("wrong persisted token digest")
	}
	var persisted string
	db.Pool.QueryRow(ctx, "SELECT row_to_json(t)::text FROM auth_action_tokens t").Scan(&persisted)
	if strings.Contains(persisted, d.Secret.Encoded()) {
		t.Fatal("raw bearer persisted")
	}
	jsonData, _ := json.Marshal(d)
	if string(jsonData) != "{}" || strings.Contains(fmt.Sprintf("%+v", d), d.Secret.Encoded()) {
		t.Fatal("delivery leaked")
	}
	if _, err := repo.TokenSubject(ctx, d.Secret.Encoded(), actiontoken.ResetPassword); !errors.Is(err, ErrActionToken) {
		t.Fatal("purpose confusion")
	}
	db.Pool.Exec(ctx, "UPDATE auth_action_tokens SET created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 hour'")
	if err := repo.ConfirmEmail(ctx, d.Secret.Encoded()); !errors.Is(err, ErrActionToken) {
		t.Fatal("expired action accepted")
	}
	db.Pool.Exec(ctx, "UPDATE auth_action_tokens SET created_at=clock_timestamp()-interval '1 second',expires_at=clock_timestamp()+interval '1 hour'")
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; results <- repo.ConfirmEmail(ctx, d.Secret.Encoded()) }()
	}
	close(start)
	wg.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrActionToken) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("token consumed more than once")
	}
	confirmed, err := repo.ByID(ctx, original.User.ID)
	if err != nil || confirmed.User.EmailVerifiedAt == nil {
		t.Fatal("email not confirmed")
	}
	reset := requestDelivery(t, repo, original.User.Email, actiontoken.ResetPassword)
	_, current := userTestHashers(t)
	v, err := current.Verify(ctx, accountTestPassword, confirmed.Credential)
	if err != nil || v.Replacement == nil {
		t.Fatal("rotation fixture")
	}
	if ok, err := repo.Upgrade(ctx, confirmed, *v.Replacement); err != nil || !ok {
		t.Fatal("rehash failed")
	}
	if _, err := repo.TokenSubject(ctx, reset.Secret.Encoded(), actiontoken.ResetPassword); err != nil {
		t.Fatal("rehash invalidated password-version token")
	}
	next, err := current.Hash(ctx, changedTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	start = make(chan struct{})
	results = make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); <-start; results <- repo.ResetPassword(ctx, reset.Secret.Encoded(), next) }()
	}
	close(start)
	wg.Wait()
	close(results)
	successes = 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrActionToken) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatal("reset token reused")
	}
	final, _ := repo.ByID(ctx, original.User.ID)
	if final.PasswordVersion != 2 {
		t.Fatal("reset did not revoke old password version")
	}
	for value, want := range map[string]bool{accountTestPassword: false, changedTestPassword: true} {
		v, err := current.Verify(ctx, value, final.Credential)
		if err != nil || v.Match != want {
			t.Fatal("wrong post-reset credential")
		}
	}
	if _, err := repo.TokenSubject(ctx, reset.Secret.Encoded(), actiontoken.ResetPassword); !errors.Is(err, ErrActionToken) {
		t.Fatal("reset replay accepted")
	}
}
func TestPostgresActionRollbackAndStaleDelivery(t *testing.T) {
	db, repo, user := actionFixture(t)
	ctx := context.Background()
	d := requestDelivery(t, repo, user.User.Email, actiontoken.ConfirmEmail)
	rollback := errors.New("rollback fixture")
	if err := db.WithUsers(ctx, func(tx *UserStore) error {
		if err := tx.ConfirmEmail(ctx, d.Secret.Encoded()); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatal("transaction did not roll back")
	}
	if _, err := repo.TokenSubject(ctx, d.Secret.Encoded(), d.Purpose); err != nil {
		t.Fatal("rollback consumed token")
	}
	db.Pool.Exec(ctx, "UPDATE auth_mail_jobs SET lease_until=clock_timestamp()-interval '1 second'")
	retry, err := repo.ClaimDelivery(ctx)
	if err != nil || retry == nil || retry.Secret.Encoded() == d.Secret.Encoded() {
		t.Fatal("retry did not replace raw token")
	}
	if ok, err := repo.FinishDelivery(ctx, *d, true); err != nil || ok {
		t.Fatal("stale lease completed new attempt")
	}
	if _, err := repo.TokenSubject(ctx, d.Secret.Encoded(), d.Purpose); !errors.Is(err, ErrActionToken) {
		t.Fatal("superseded token remains valid")
	}
	if ok, err := repo.FinishDelivery(ctx, *retry, false); err != nil || !ok {
		t.Fatal("send failure not recorded")
	}
	if next, err := repo.ClaimDelivery(ctx); err != nil || next != nil {
		t.Fatal("backoff ignored")
	}
	if _, err := repo.TokenSubject(ctx, retry.Secret.Encoded(), retry.Purpose); !errors.Is(err, ErrActionToken) {
		t.Fatal("failed email token remains valid")
	}
	db.Pool.Exec(ctx, "UPDATE auth_mail_jobs SET attempts=4,available_at=clock_timestamp()-interval '1 second'")
	last, err := repo.ClaimDelivery(ctx)
	if err != nil || last == nil {
		t.Fatal("last attempt unavailable")
	}
	db.Pool.Exec(ctx, "UPDATE auth_mail_jobs SET lease_until=clock_timestamp()-interval '1 second'")
	if next, err := repo.ClaimDelivery(ctx); err != nil || next != nil {
		t.Fatal("exhausted attempt retried")
	}
	var status string
	db.Pool.QueryRow(ctx, "SELECT status FROM auth_mail_jobs").Scan(&status)
	if status != "failed" {
		t.Fatal("crashed final attempt blocks queue forever")
	}
}
func TestPostgresAccountActionRateAndAddressBinding(t *testing.T) {
	db, repo, user := actionFixture(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		d := requestDelivery(t, repo, user.User.Email, actiontoken.ConfirmEmail)
		if ok, err := repo.FinishDelivery(ctx, *d, true); err != nil || !ok {
			t.Fatal("delivery ack failed")
		}
		db.Pool.Exec(ctx, "UPDATE auth_mail_jobs SET created_at=clock_timestamp()-interval '2 minutes'")
	}
	if err := repo.RequestAction(ctx, user.User.Email, actiontoken.ConfirmEmail); err != nil {
		t.Fatal(err)
	}
	if d, err := repo.ClaimDelivery(ctx); err != nil || d != nil {
		t.Fatal("hourly limit bypassed")
	}
	db.Pool.Exec(ctx, "UPDATE auth_mail_jobs SET created_at=clock_timestamp()-interval '2 hours'")
	d := requestDelivery(t, repo, user.User.Email, actiontoken.ConfirmEmail)
	db.Pool.Exec(ctx, "UPDATE auth_users SET email='changed@example.test' WHERE id=$1", user.User.ID)
	if err := repo.ConfirmEmail(ctx, d.Secret.Encoded()); !errors.Is(err, ErrActionToken) {
		t.Fatal("old address token confirmed new address")
	}
}

func TestPostgresResetRollbackVersionBindingAndExpiredIntent(t *testing.T) {
	db, repo, user := actionFixture(t)
	ctx := context.Background()
	confirmation := requestDelivery(t, repo, user.User.Email, actiontoken.ConfirmEmail)
	if err := repo.ConfirmEmail(ctx, confirmation.Secret.Encoded()); err != nil {
		t.Fatal(err)
	}
	reset := requestDelivery(t, repo, user.User.Email, actiontoken.ResetPassword)
	old, _ := userTestHashers(t)
	next, err := old.Hash(ctx, changedTestPassword)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("rollback reset fixture")
	err = db.WithUsers(ctx, func(tx *UserStore) error {
		if err := tx.ResetPassword(ctx, reset.Secret.Encoded(), next); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal("reset transaction failed")
	}
	unchanged, _ := repo.ByID(ctx, user.User.ID)
	if unchanged.PasswordVersion != 1 || unchanged.Credential.PHC != user.Credential.PHC {
		t.Fatal("failed reset changed credentials")
	}
	if _, err = repo.TokenSubject(ctx, reset.Secret.Encoded(), actiontoken.ResetPassword); err != nil {
		t.Fatal("rollback consumed reset token")
	}
	if ok, err := repo.ChangePassword(ctx, unchanged, next); err != nil || !ok {
		t.Fatal("independent password change failed")
	}
	if err = repo.ResetPassword(ctx, reset.Secret.Encoded(), next); !errors.Is(err, ErrActionToken) {
		t.Fatal("old-version reset token accepted")
	}
	db.Pool.Exec(ctx, "UPDATE auth_mail_jobs SET lease_until=CASE WHEN status='sending' THEN clock_timestamp()-interval '1 second' ELSE NULL END,created_at=clock_timestamp()-interval '2 hours',expires_at=clock_timestamp()-interval '1 hour' WHERE status IN ('pending','sending')")
	if d, err := repo.ClaimDelivery(ctx); err != nil || d != nil {
		t.Fatal("expired intent delivered")
	}
}
