package store

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"devcourse-finder/auth/password"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrUserInput     = errors.New("invalid user storage input")
	ErrUserNotFound  = errors.New("user credential not found")
	ErrEmailConflict = errors.New("email already reserved")
	ErrUserStorage   = errors.New("user credential storage failed")
)
var userIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// NormalizeEmail defines account identity, not mail-provider aliasing. It accepts
// bare ASCII dot-atom addresses, folds ASCII case and trims surrounding space.
// International domains must already be IDNA ASCII. No plus/dot alias rewriting.
func NormalizeEmail(raw string) (string, error) {
	if len(raw) > 1024 {
		return "", ErrUserInput
	}
	email := strings.TrimSpace(raw)
	for _, b := range []byte(email) {
		if b >= 128 {
			return "", ErrUserInput
		}
	}
	email = strings.ToLower(email)
	if len(email) > 254 || strings.Count(email, "@") != 1 {
		return "", ErrUserInput
	}
	local, domain, _ := strings.Cut(email, "@")
	if len(local) < 1 || len(local) > 64 || len(domain) < 3 || strings.HasPrefix(local, ".") || strings.HasSuffix(local, ".") || strings.Contains(local, "..") {
		return "", ErrUserInput
	}
	for _, c := range local {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '.' || strings.ContainsRune("!#$%&'*+-/=?^_`{|}~", c)) {
			return "", ErrUserInput
		}
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return "", ErrUserInput
	}
	for _, label := range labels {
		if len(label) < 1 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", ErrUserInput
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return "", ErrUserInput
			}
		}
	}
	return email, nil
}

// User and UserCredential are private storage records, not response DTOs.
type User struct {
	ID              string     `json:"-"`
	Email           string     `json:"-"`
	EmailVerifiedAt *time.Time `json:"-"`
	CreatedAt       time.Time  `json:"-"`
}

func (User) String() string     { return "store.User(redacted)" }
func (u User) GoString() string { return u.String() }

type UserCredential struct {
	User              User                `json:"-"`
	Credential        password.Credential `json:"-"`
	Revision          int64               `json:"-"`
	PasswordVersion   int64               `json:"-"`
	PasswordChangedAt time.Time           `json:"-"`
}

func (UserCredential) String() string     { return "store.UserCredential(redacted)" }
func (c UserCredential) GoString() string { return c.String() }

type userQueries interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

// UserStore retains only a pool/transaction, never passwords or pepper bytes.
type UserStore struct{ q userQueries }

func (*UserStore) String() string     { return "store.UserStore(redacted)" }
func (s *UserStore) GoString() string { return s.String() }

func (d *DB) Users() *UserStore { return &UserStore{q: d.Pool} }

// WithUsers permits future token consumption and password replacement in the
// same transaction. Create uses a nested savepoint when called inside this scope.
func (d *DB) WithUsers(ctx context.Context, fn func(*UserStore) error) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return ErrUserStorage
	}
	defer tx.Rollback(ctx)
	if err = fn(&UserStore{q: tx}); err != nil {
		return err
	}
	if tx.Commit(ctx) != nil {
		return ErrUserStorage
	}
	return nil
}
func userStorageError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUserNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "auth_users_email_key" {
		return ErrEmailConflict
	}
	return ErrUserStorage
}
func newUserID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", ErrUserStorage
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}

func (s *UserStore) Create(ctx context.Context, email string, c password.Credential) (User, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil || password.ValidateCredential(c) != nil {
		return User{}, ErrUserInput
	}
	id, err := newUserID()
	if err != nil {
		return User{}, err
	}
	tx, err := s.q.Begin(ctx)
	if err != nil {
		return User{}, ErrUserStorage
	}
	defer tx.Rollback(ctx)
	u := User{ID: id, Email: normalized}
	err = tx.QueryRow(ctx, `INSERT INTO auth_users(id,email) VALUES($1,$2) RETURNING created_at`, id, normalized).Scan(&u.CreatedAt)
	if err != nil {
		return User{}, userStorageError(err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO auth_credentials(user_id,scheme,phc,pepper_key_id) VALUES($1,$2,$3,$4)`, id, c.Scheme, c.PHC, c.PepperKeyID)
	if err != nil {
		return User{}, ErrUserStorage
	}
	if tx.Commit(ctx) != nil {
		return User{}, ErrUserStorage
	}
	return u, nil
}

func (s *UserStore) ByEmail(ctx context.Context, email string) (UserCredential, error) {
	normalized, err := NormalizeEmail(email)
	if err != nil {
		return UserCredential{}, ErrUserInput
	}
	return s.lookup(ctx, "u.email=$1", normalized)
}
func (s *UserStore) ByID(ctx context.Context, id string) (UserCredential, error) {
	if !userIDPattern.MatchString(id) {
		return UserCredential{}, ErrUserInput
	}
	return s.lookup(ctx, "u.id=$1", id)
}
func (s *UserStore) lookup(ctx context.Context, predicate string, value string) (UserCredential, error) {
	var c UserCredential
	// predicate comes only from the two fixed call sites, never user input.
	err := s.q.QueryRow(ctx, `SELECT u.id::text,u.email,u.email_verified_at,u.created_at,c.scheme,c.phc,c.pepper_key_id,c.revision,c.password_version,c.password_changed_at FROM auth_users u JOIN auth_credentials c ON c.user_id=u.id WHERE `+predicate, value).Scan(&c.User.ID, &c.User.Email, &c.User.EmailVerifiedAt, &c.User.CreatedAt, &c.Credential.Scheme, &c.Credential.PHC, &c.Credential.PepperKeyID, &c.Revision, &c.PasswordVersion, &c.PasswordChangedAt)
	if err != nil {
		return UserCredential{}, userStorageError(err)
	}
	if password.ValidateCredential(c.Credential) != nil {
		return UserCredential{}, ErrUserStorage
	}
	return c, nil
}

// Upgrade replaces a verified credential without changing password age/version.
// False means the observed row changed; never retry using stale authorization.
func (s *UserStore) Upgrade(ctx context.Context, expected UserCredential, next password.Credential) (bool, error) {
	return s.replace(ctx, expected, next, false)
}

// ChangePassword requires an expected revision. A future reset-token service
// must consume its token in the same WithUsers transaction and revoke sessions.
func (s *UserStore) ChangePassword(ctx context.Context, expected UserCredential, next password.Credential) (bool, error) {
	return s.replace(ctx, expected, next, true)
}
func (s *UserStore) replace(ctx context.Context, old UserCredential, next password.Credential, changed bool) (bool, error) {
	if !userIDPattern.MatchString(old.User.ID) || old.Revision < 1 || old.PasswordVersion < 1 || password.ValidateCredential(old.Credential) != nil || password.ValidateCredential(next) != nil {
		return false, ErrUserInput
	}
	result, err := s.q.Exec(ctx, `UPDATE auth_credentials SET scheme=$1,phc=$2,pepper_key_id=$3,revision=revision+1,password_version=password_version+CASE WHEN $4 THEN 1 ELSE 0 END,password_changed_at=CASE WHEN $4 THEN clock_timestamp() ELSE password_changed_at END WHERE user_id=$5 AND revision=$6 AND password_version=$7 AND scheme=$8 AND phc=$9 AND pepper_key_id=$10`, next.Scheme, next.PHC, next.PepperKeyID, changed, old.User.ID, old.Revision, old.PasswordVersion, old.Credential.Scheme, old.Credential.PHC, old.Credential.PepperKeyID)
	if err != nil {
		return false, ErrUserStorage
	}
	return result.RowsAffected() == 1, nil
}
