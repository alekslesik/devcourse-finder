package store

import (
	"context"
	"errors"
	"time"

	"devcourse-finder/auth/actiontoken"
	"devcourse-finder/auth/password"
	"github.com/jackc/pgx/v5"
)

var ErrActionToken = errors.New("account action unavailable or invalid")

// lockUser establishes the common lock order: user, credential, job/token.
// All credential changes and account actions must acquire this lock first.
func (s *UserStore) lockUser(ctx context.Context, id string) error {
	var found string
	if err := s.q.QueryRow(ctx, "SELECT id::text FROM auth_users WHERE id=$1 FOR UPDATE", id).Scan(&found); err != nil {
		return userStorageError(err)
	}
	return nil
}
func eligibleAction(c UserCredential, p actiontoken.Purpose) bool {
	return p == actiontoken.ConfirmEmail && c.User.EmailVerifiedAt == nil || p == actiontoken.ResetPassword && c.User.EmailVerifiedAt != nil
}

// RequestAction has the same successful result for unknown, ineligible, queued
// and throttled accounts. HTTP/global/IP controls remain mandatory at activation.
func (s *UserStore) RequestAction(ctx context.Context, email string, p actiontoken.Purpose) error {
	if !p.Valid() {
		return ErrUserInput
	}
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return nil
	}
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return ErrUserStorage
	}
	defer tx.Rollback(ctx)
	repo := &UserStore{q: tx}
	c, err := repo.ByEmail(ctx, normalized)
	if errors.Is(err, ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err = repo.lockUser(ctx, c.User.ID); errors.Is(err, ErrUserNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	c, err = repo.ByID(ctx, c.User.ID)
	if errors.Is(err, ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !eligibleAction(c, p) {
		return nil
	}
	// A final crashed attempt must not reserve the unique active job forever.
	_, err = tx.Exec(ctx, `UPDATE auth_mail_jobs SET status='failed',lease_id=NULL,lease_until=NULL WHERE user_id=$1 AND ((status='sending' AND lease_until<=clock_timestamp() AND (attempts>=5 OR expires_at<=clock_timestamp())) OR (status='pending' AND expires_at<=clock_timestamp()))`, c.User.ID)
	if err != nil {
		return ErrUserStorage
	}
	var blocked bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth_mail_jobs WHERE user_id=$1 AND purpose=$2 AND (status IN ('pending','sending') OR created_at>clock_timestamp()-interval '60 seconds')) OR (SELECT count(*) FROM auth_mail_jobs WHERE user_id=$1 AND purpose=$2 AND created_at>clock_timestamp()-interval '1 hour')>=5`, c.User.ID, string(p)).Scan(&blocked)
	if err != nil {
		return ErrUserStorage
	}
	if blocked {
		return nil
	}
	id, err := newUserID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO auth_mail_jobs(id,user_id,email,purpose,password_version,expires_at) VALUES($1,$2,$3,$4,$5,clock_timestamp()+$6::bigint*interval '1 second')`, id, c.User.ID, c.User.Email, string(p), c.PasswordVersion, int64(p.TTL()/time.Second))
	if err != nil {
		return ErrUserStorage
	}
	if tx.Commit(ctx) != nil {
		return ErrUserStorage
	}
	return nil
}

// Delivery is an in-memory attempt, not a persisted email body or response DTO.
type Delivery struct {
	JobID   string              `json:"-"`
	LeaseID string              `json:"-"`
	UserID  string              `json:"-"`
	Email   string              `json:"-"`
	Purpose actiontoken.Purpose `json:"-"`
	Secret  actiontoken.Secret  `json:"-"`
}

func (Delivery) String() string     { return "store.Delivery(redacted)" }
func (d Delivery) GoString() string { return d.String() }

// ClaimDelivery creates a new secret for each attempt. Only its digest is stored.
func (s *UserStore) ClaimDelivery(ctx context.Context) (*Delivery, error) {
	_, err := s.q.Exec(ctx, `UPDATE auth_mail_jobs SET status='failed',lease_id=NULL,lease_until=NULL WHERE ((status='sending' AND lease_until<=clock_timestamp() AND (attempts>=5 OR expires_at<=clock_timestamp())) OR (status='pending' AND expires_at<=clock_timestamp()))`)
	if err != nil {
		return nil, ErrUserStorage
	}
	var id, uid string
	err = s.q.QueryRow(ctx, `SELECT id::text,user_id::text FROM auth_mail_jobs WHERE attempts<5 AND expires_at>clock_timestamp() AND ((status='pending' AND available_at<=clock_timestamp()) OR (status='sending' AND lease_until<=clock_timestamp())) ORDER BY created_at,id LIMIT 1`).Scan(&id, &uid)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrUserStorage
	}
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return nil, ErrUserStorage
	}
	defer tx.Rollback(ctx)
	repo := &UserStore{q: tx}
	if err = repo.lockUser(ctx, uid); errors.Is(err, ErrUserNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	c, err := repo.ByID(ctx, uid)
	if err != nil {
		return nil, err
	}
	var email, purpose string
	var version int64
	err = tx.QueryRow(ctx, `SELECT email,purpose,password_version FROM auth_mail_jobs WHERE id=$1 AND attempts<5 AND expires_at>clock_timestamp() AND ((status='pending' AND available_at<=clock_timestamp()) OR (status='sending' AND lease_until<=clock_timestamp())) FOR UPDATE SKIP LOCKED`, id).Scan(&email, &purpose, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrUserStorage
	}
	p := actiontoken.Purpose(purpose)
	if email != c.User.Email || version != c.PasswordVersion || !eligibleAction(c, p) {
		if _, err = tx.Exec(ctx, `UPDATE auth_mail_jobs SET status='superseded',lease_id=NULL,lease_until=NULL WHERE id=$1`, id); err != nil {
			return nil, ErrUserStorage
		}
		if tx.Commit(ctx) != nil {
			return nil, ErrUserStorage
		}
		return nil, nil
	}
	secret, err := actiontoken.Generate()
	if err != nil {
		return nil, ErrActionToken
	}
	digest, _ := secret.Digest(p)
	lease, err := newUserID()
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, `UPDATE auth_action_tokens SET consumed_at=clock_timestamp() WHERE user_id=$1 AND purpose=$2 AND consumed_at IS NULL`, uid, purpose); err != nil {
		return nil, ErrUserStorage
	}
	_, err = tx.Exec(ctx, `INSERT INTO auth_action_tokens(digest,user_id,email,purpose,password_version,mail_job_id,expires_at) VALUES($1,$2,$3,$4,$5,$6,clock_timestamp()+$7::bigint*interval '1 second')`, digest[:], uid, email, purpose, version, id, int64(p.TTL()/time.Second))
	if err != nil {
		return nil, ErrUserStorage
	}
	_, err = tx.Exec(ctx, `UPDATE auth_mail_jobs SET status='sending',attempts=attempts+1,lease_id=$2,lease_until=clock_timestamp()+interval '2 minutes',token_digest=$3 WHERE id=$1`, id, lease, digest[:])
	if err != nil {
		return nil, ErrUserStorage
	}
	if tx.Commit(ctx) != nil {
		return nil, ErrUserStorage
	}
	return &Delivery{JobID: id, LeaseID: lease, UserID: uid, Email: email, Purpose: p, Secret: secret}, nil
}

// FinishDelivery is fenced by lease ID and digest; stale workers cannot update
// a new attempt. SMTP retries are at-least-once and may invalidate an older link.
func (s *UserStore) FinishDelivery(ctx context.Context, d Delivery, sent bool) (bool, error) {
	if !userIDPattern.MatchString(d.JobID) || !userIDPattern.MatchString(d.LeaseID) || !userIDPattern.MatchString(d.UserID) || !d.Purpose.Valid() {
		return false, ErrUserInput
	}
	digest, _ := d.Secret.Digest(d.Purpose)
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return false, ErrUserStorage
	}
	defer tx.Rollback(ctx)
	repo := &UserStore{q: tx}
	if err = repo.lockUser(ctx, d.UserID); errors.Is(err, ErrUserNotFound) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	var attempts int
	err = tx.QueryRow(ctx, `SELECT attempts FROM auth_mail_jobs WHERE id=$1 AND user_id=$2 AND lease_id=$3 AND token_digest=$4 AND status='sending' AND lease_until>clock_timestamp() FOR UPDATE`, d.JobID, d.UserID, d.LeaseID, digest[:]).Scan(&attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, ErrUserStorage
	}
	status := "sent"
	if !sent {
		status = "pending"
		if attempts >= 5 {
			status = "failed"
		}
	}
	_, err = tx.Exec(ctx, `UPDATE auth_mail_jobs SET status=$2,lease_id=NULL,lease_until=NULL,available_at=clock_timestamp()+$3::bigint*interval '1 second' WHERE id=$1`, d.JobID, status, int64(30*(1<<uint(attempts-1))))
	if err != nil {
		return false, ErrUserStorage
	}
	if !sent {
		if _, err = tx.Exec(ctx, `UPDATE auth_action_tokens SET consumed_at=clock_timestamp() WHERE digest=$1 AND consumed_at IS NULL`, digest[:]); err != nil {
			return false, ErrUserStorage
		}
	}
	if tx.Commit(ctx) != nil {
		return false, ErrUserStorage
	}
	return true, nil
}

// TokenSubject is a private preflight before a bounded password KDF. Consumption
// rechecks every condition in its transaction; this read never authorizes a reset.
func (s *UserStore) TokenSubject(ctx context.Context, raw string, p actiontoken.Purpose) (UserCredential, error) {
	secret, err := actiontoken.Parse(raw)
	if err != nil || !p.Valid() {
		return UserCredential{}, ErrActionToken
	}
	digest, _ := secret.Digest(p)
	var uid, email string
	var version int64
	err = s.q.QueryRow(ctx, `SELECT user_id::text,email,password_version FROM auth_action_tokens WHERE digest=$1 AND purpose=$2 AND consumed_at IS NULL AND expires_at>clock_timestamp()`, digest[:], string(p)).Scan(&uid, &email, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserCredential{}, ErrActionToken
	}
	if err != nil {
		return UserCredential{}, ErrUserStorage
	}
	c, err := s.ByID(ctx, uid)
	if errors.Is(err, ErrUserNotFound) {
		return UserCredential{}, ErrActionToken
	}
	if err != nil {
		return UserCredential{}, err
	}
	if c.User.Email != email || c.PasswordVersion != version || !eligibleAction(c, p) {
		return UserCredential{}, ErrActionToken
	}
	return c, nil
}
func (s *UserStore) ConfirmEmail(ctx context.Context, raw string) error {
	return s.consumeAction(ctx, raw, actiontoken.ConfirmEmail, password.Credential{})
}
func (s *UserStore) ResetPassword(ctx context.Context, raw string, next password.Credential) error {
	if password.ValidateCredential(next) != nil {
		return ErrUserInput
	}
	return s.consumeAction(ctx, raw, actiontoken.ResetPassword, next)
}
func (s *UserStore) consumeAction(ctx context.Context, raw string, p actiontoken.Purpose, next password.Credential) error {
	subject, err := s.TokenSubject(ctx, raw, p)
	if err != nil {
		return err
	}
	secret, _ := actiontoken.Parse(raw)
	digest, _ := secret.Digest(p)
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return ErrUserStorage
	}
	defer tx.Rollback(ctx)
	repo := &UserStore{q: tx}
	if err = repo.lockUser(ctx, subject.User.ID); err != nil {
		return ErrActionToken
	}
	current, err := repo.TokenSubject(ctx, raw, p)
	if err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE auth_action_tokens SET consumed_at=clock_timestamp() WHERE digest=$1 AND consumed_at IS NULL AND expires_at>clock_timestamp()`, digest[:])
	if err != nil {
		return ErrUserStorage
	}
	if result.RowsAffected() != 1 {
		return ErrActionToken
	}
	if p == actiontoken.ConfirmEmail {
		if _, err = tx.Exec(ctx, `UPDATE auth_users SET email_verified_at=clock_timestamp() WHERE id=$1 AND email_verified_at IS NULL`, current.User.ID); err != nil {
			return ErrUserStorage
		}
	} else {
		ok, err := repo.ChangePassword(ctx, current, next)
		if err != nil {
			return err
		}
		if !ok {
			return ErrActionToken
		}
	}
	// A password reset invalidates every old-version action. Confirmation cancels
	// only confirmations; unrelated valid reset requests remain independently bound.
	_, err = tx.Exec(ctx, `UPDATE auth_action_tokens SET consumed_at=clock_timestamp() WHERE user_id=$1 AND consumed_at IS NULL AND ($2 OR purpose=$3)`, current.User.ID, p == actiontoken.ResetPassword, string(p))
	if err != nil {
		return ErrUserStorage
	}
	_, err = tx.Exec(ctx, `UPDATE auth_mail_jobs SET status='superseded',lease_id=NULL,lease_until=NULL WHERE user_id=$1 AND status IN ('pending','sending') AND ($2 OR purpose=$3)`, current.User.ID, p == actiontoken.ResetPassword, string(p))
	if err != nil {
		return ErrUserStorage
	}
	if tx.Commit(ctx) != nil {
		return ErrUserStorage
	}
	return nil
}
