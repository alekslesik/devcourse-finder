package main

import (
	"context"
	"devcourse-finder/auth/accountmail"
	"devcourse-finder/auth/actionhttp"
	"devcourse-finder/auth/actiontoken"
	"devcourse-finder/auth/password"
	"devcourse-finder/store"
	"encoding/base64"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func accountHTTP(db *store.DB) (http.Handler, error) {
	switch os.Getenv("AUTH_ACTIONS_ENABLED") {
	case "", "false":
		return nil, nil
	case "true":
	default:
		return nil, actionhttp.ErrConfig
	}
	key, err := base64.StdEncoding.Strict().DecodeString(os.Getenv("AUTH_GATEWAY_SECRET"))
	if err != nil || len(key) != 32 || base64.StdEncoding.EncodeToString(key) != os.Getenv("AUTH_GATEWAY_SECRET") {
		return nil, actionhttp.ErrConfig
	}
	origin, err := actiontoken.NewOrigin(os.Getenv("AUTH_PUBLIC_ORIGIN"))
	if err != nil {
		return nil, actionhttp.ErrConfig
	}
	ring, err := password.LoadKeyring(os.Getenv("PASSWORD_KEYRING_FILE"))
	if err != nil {
		return nil, actionhttp.ErrConfig
	}
	h, err := ring.Hasher(password.DefaultParams(), 1)
	if err != nil {
		return nil, err
	}
	service, err := accountmail.NewHTTP(db.Users(), origin, h)
	if err != nil {
		return nil, err
	}
	return actionhttp.New(service, key, os.Getenv("AUTH_PUBLIC_ORIGIN"))
}
func accountWorker(ctx context.Context, db *store.DB, mode string) error {
	if os.Getenv("AUTH_MAIL_ENABLED") != "true" {
		return commandFailure("auth_mail", "mail_disabled", "Account mail worker is disabled", nil)
	}
	service, err := accountmail.NewWorker(db.Users(), os.Getenv("AUTH_SMTP_FILE"), os.Getenv("PASSWORD_KEYRING_FILE"))
	if err != nil {
		return commandFailure("auth_mail", "invalid_mail_configuration", "Account mail configuration unavailable; values hidden", nil)
	}
	if mode == "health" {
		stamp, readErr := os.ReadFile("/tmp/dcf-account-mail.heartbeat")
		if readErr != nil || len(stamp) > 32 {
			return commandFailure("auth_mail", "mail_unhealthy", "Account mail worker heartbeat unavailable", nil)
		}
		unix, parseErr := strconv.ParseInt(strings.TrimSpace(string(stamp)), 10, 64)
		age := time.Since(time.Unix(unix, 0))
		if parseErr != nil || age < 0 || age > 90*time.Second {
			return commandFailure("auth_mail", "mail_unhealthy", "Account mail worker heartbeat is stale", nil)
		}
		bounded, done := context.WithTimeout(ctx, 5*time.Second)
		defer done()
		if db.Ping(bounded) != nil {
			return commandFailure("auth_mail", "mail_unhealthy", "Account mail database unavailable", nil)
		}
		return nil
	}
	if mode == "once" {
		_, err := service.SendOne(ctx)
		if err != nil {
			return commandFailure("auth_mail", "delivery_failed", "Account mail attempt failed; values hidden", nil)
		}
		return nil
	}
	stop, done := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer done()
	for {
		if err := os.WriteFile("/tmp/dcf-account-mail.heartbeat", []byte(strconv.FormatInt(time.Now().Unix(), 10)), 0600); err != nil {
			return commandFailure("auth_mail", "mail_unhealthy", "Account mail heartbeat failed", nil)
		}
		bounded, cancel := context.WithTimeout(stop, 30*time.Second)
		_, err := service.SendOne(bounded)
		cancel()
		// Delivery status/backoff is persisted; do not log provider errors or messages.
		if err != nil && stop.Err() == nil {
			slog.Warn("account mail attempt failed", "code", "delivery_unavailable")
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-stop.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
