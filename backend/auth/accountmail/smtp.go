package accountmail

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"mime"
	"net"
	"net/smtp"
	"net/url"
	"strings"
	"time"

	"devcourse-finder/auth/actiontoken"
	"devcourse-finder/internal/privatejson"
	"devcourse-finder/store"
)

type smtpSettings struct {
	PublicOrigin string `json:"public_origin"`
	Version      int    `json:"version"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	Username     string `json:"username"`
	From         string `json:"from"`
	Password     string `json:"password"`
}
type SMTP struct {
	origin    actiontoken.Origin
	settings  smtpSettings
	dial      func(context.Context, string) (net.Conn, error)
	tlsConfig *tls.Config
}

func (*SMTP) String() string     { return "accountmail.SMTP(redacted)" }
func (s *SMTP) GoString() string { return s.String() }
func LoadSMTP(path string) (*SMTP, error) {
	data, err := privatejson.Read(path)
	if err != nil {
		return nil, ErrConfiguration
	}
	defer clear(data)
	var c smtpSettings
	if privatejson.Decode(data, &c, "version", "host", "port", "username", "from", "password", "public_origin") != nil {
		return nil, ErrConfiguration
	}
	origin, err := actiontoken.NewOrigin(c.PublicOrigin)
	if err != nil {
		return nil, ErrConfiguration
	}
	user, err := store.NormalizeEmail(c.Username)
	if err != nil || user != c.Username || c.From != user || c.Version != 1 || !validSMTPHost(c.Host) || (c.Port != 465 && c.Port != 587) || len(c.Password) < 1 || len(c.Password) > 512 || strings.ContainsAny(c.Password, "\r\n\x00") {
		return nil, ErrConfiguration
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	return &SMTP{origin: origin, settings: c, dial: func(ctx context.Context, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", address)
	}, tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: c.Host}}, nil
}

// SMTP endpoints are chosen only in an operator-owned private configuration.
// A canonical DNS name pins TLS SNI and certificate verification; URL/IP/port
// syntax and plaintext SMTP ports are not accepted here.
func validSMTPHost(host string) bool {
	if len(host) > 253 || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
				return false
			}
		}
	}
	return true
}

func (s *SMTP) Send(ctx context.Context, m Message) error {
	to, err := store.NormalizeEmail(m.To)
	if err != nil || to != m.To || !m.Purpose.Valid() {
		return ErrTransport
	}
	// Only trusted fragment links may be sent; never accept CR/LF or a query token.
	u, parseErr := url.Parse(m.Link)
	if parseErr != nil {
		return ErrTransport
	}
	fragment, parseErr := url.ParseQuery(u.Fragment)
	if parseErr != nil || len(fragment) != 1 || len(fragment["token"]) != 1 {
		return ErrTransport
	}
	secret, parseErr := actiontoken.Parse(fragment.Get("token"))
	if parseErr != nil {
		return ErrTransport
	}
	expected, parseErr := s.origin.Link(m.Purpose, secret)
	if parseErr != nil || m.Link != expected {
		return ErrTransport
	}
	conn, err := s.dial(ctx, net.JoinHostPort(s.settings.Host, fmt.Sprint(s.settings.Port)))
	if err != nil {
		return ErrTransport
	}
	defer conn.Close()
	deadline := time.Now().Add(15 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if conn.SetDeadline(deadline) != nil {
		return ErrTransport
	}
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { rawConn.Close() })
	defer stop()
	if s.settings.Port == 465 {
		secured := tls.Client(conn, s.tlsConfig.Clone())
		if secured.HandshakeContext(ctx) != nil {
			return ErrTransport
		}
		conn = secured
	}
	client, err := smtp.NewClient(conn, s.settings.Host)
	if err != nil {
		return ErrTransport
	}
	defer client.Close()
	if s.settings.Port == 587 {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return ErrTransport
		}
		if client.StartTLS(s.tlsConfig.Clone()) != nil {
			return ErrTransport
		}
	}
	if client.Auth(smtp.PlainAuth("", s.settings.Username, s.settings.Password, s.settings.Host)) != nil {
		return ErrTransport
	}
	if client.Mail(s.settings.From) != nil || client.Rcpt(to) != nil {
		return ErrTransport
	}
	data, err := client.Data()
	if err != nil {
		return ErrTransport
	}
	subject := "Подтверждение email"
	body := "Подтвердите адрес электронной почты по ссылке. Ссылка действует 24 часа."
	if m.Purpose == actiontoken.ResetPassword {
		subject = "Сброс пароля"
		body = "Перейдите по ссылке, чтобы задать новый пароль. Ссылка действует 15 минут."
	}
	body += "\n\n" + m.Link + "\n\nЕсли вы не запрашивали это действие, проигнорируйте письмо.\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	var wrapped strings.Builder
	for len(encoded) > 76 {
		wrapped.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	wrapped.WriteString(encoded + "\r\n")
	var messageID [16]byte
	if _, err := rand.Read(messageID[:]); err != nil {
		return ErrTransport
	}
	headers := fmt.Sprintf("Date: %s\r\nMessage-ID: <%x@%s>\r\n", time.Now().UTC().Format(time.RFC1123Z), messageID, u.Hostname())
	message := headers + "From: <" + s.settings.From + ">\r\nTo: <" + to + ">\r\nSubject: " + mime.QEncoding.Encode("utf-8", subject) + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n" + wrapped.String()
	if _, err = data.Write([]byte(message)); err != nil {
		return ErrTransport
	}
	if data.Close() != nil {
		return ErrTransport
	}
	// DATA acceptance is the delivery boundary. A failed QUIT must not resend it.
	_ = client.Quit()
	return nil
}
