// Package accountmail delivers private account-action intents. It exposes no HTTP routes.
package accountmail

import (
	"context"
	"devcourse-finder/auth/actiontoken"
	"devcourse-finder/auth/password"
	"devcourse-finder/store"
	"errors"
	"time"
)

var ErrTransport = errors.New("account email delivery failed")
var ErrConfiguration = errors.New("account mail configuration invalid")

type Message struct {
	To      string              `json:"-"`
	Purpose actiontoken.Purpose `json:"-"`
	Link    string              `json:"-"`
}

func (Message) String() string     { return "accountmail.Message(redacted)" }
func (m Message) GoString() string { return m.String() }

type Sender interface {
	Send(context.Context, Message) error
}

type Service struct {
	repo   *store.UserStore
	sender Sender
	origin actiontoken.Origin
	hasher *password.Hasher
	policy password.Policy
}

func (*Service) String() string     { return "accountmail.Service(redacted)" }
func (s *Service) GoString() string { return s.String() }
func New(repo *store.UserStore, sender Sender, origin actiontoken.Origin, hasher *password.Hasher) (*Service, error) {
	if repo == nil || sender == nil || hasher == nil {
		return nil, ErrConfiguration
	}
	test, _ := actiontoken.Generate()
	if _, err := origin.Link(actiontoken.ConfirmEmail, test); err != nil {
		return nil, ErrConfiguration
	}
	return &Service{repo: repo, sender: sender, origin: origin, hasher: hasher, policy: password.DefaultPolicy()}, nil
}
func (s *Service) Request(ctx context.Context, email string, p actiontoken.Purpose) error {
	return s.repo.RequestAction(ctx, email, p)
}
func (s *Service) Confirm(ctx context.Context, raw string) error {
	return s.repo.ConfirmEmail(ctx, raw)
}
func (s *Service) Reset(ctx context.Context, raw, value string) error {
	if _, err := s.repo.TokenSubject(ctx, raw, actiontoken.ResetPassword); err != nil {
		return err
	}
	if err := s.policy.Check(value); err != nil {
		return err
	}
	c, err := s.hasher.Hash(ctx, value)
	if err != nil {
		return err
	}
	return s.repo.ResetPassword(ctx, raw, c)
}

// SendOne is an explicitly invoked worker step. It never runs on API startup.
// The bounded send deadline is much shorter than the two-minute database lease.
func (s *Service) SendOne(ctx context.Context) (bool, error) {
	if s.sender == nil {
		return false, ErrConfiguration
	}
	d, err := s.repo.ClaimDelivery(ctx)
	if err != nil || d == nil {
		return false, err
	}
	link, err := s.origin.Link(d.Purpose, d.Secret)
	if err != nil {
		return false, ErrConfiguration
	}
	sendCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	err = s.sender.Send(sendCtx, Message{To: d.Email, Purpose: d.Purpose, Link: link})
	cancel()
	// Cancellation must not strand the completion of an already attempted send.
	finishCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer done()
	recorded, finishErr := s.repo.FinishDelivery(finishCtx, *d, err == nil)
	if finishErr != nil {
		return true, finishErr
	}
	if !recorded || err != nil {
		return true, ErrTransport
	}
	return true, nil
}

func NewHTTP(repo *store.UserStore, origin actiontoken.Origin, hasher *password.Hasher) (*Service, error) {
	if repo == nil || hasher == nil {
		return nil, ErrConfiguration
	}
	if _, err := origin.Link(actiontoken.ConfirmEmail, actiontoken.Secret{}); err != nil {
		return nil, ErrConfiguration
	}
	return &Service{repo: repo, origin: origin, hasher: hasher, policy: password.DefaultPolicy()}, nil
}
func NewWorker(repo *store.UserStore, smtpPath, keyringPath string) (*Service, error) {
	sender, err := LoadSMTP(smtpPath)
	if err != nil {
		return nil, err
	}
	ring, err := password.LoadKeyring(keyringPath)
	if err != nil {
		return nil, ErrConfiguration
	}
	hasher, err := ring.Hasher(password.DefaultParams(), 1)
	if err != nil {
		return nil, ErrConfiguration
	}
	return New(repo, sender, sender.origin, hasher)
}
