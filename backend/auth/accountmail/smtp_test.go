package accountmail

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"devcourse-finder/auth/actiontoken"
)

func smtpFixtureSecret(t *testing.T) string {
	t.Helper()
	var bytes [24]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		t.Fatal("fixture entropy unavailable")
	}
	return hex.EncodeToString(bytes[:])
}
func smtpFixture(t *testing.T, failAuth bool) (*SMTP, <-chan string) {
	return smtpFixtureHost(t, failAuth, "smtp.yandex.ru")
}
func smtpFixtureHost(t *testing.T, failAuth bool, host string) (*SMTP, <-chan string) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: host}, DNSNames: []string{host}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	priv, _ := x509.MarshalECPrivateKey(key)
	cert, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: priv}))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	messages := make(chan string, 1)
	go func() {
		defer close(messages)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		c := textproto.NewConn(conn)
		c.PrintfLine("220 %s fixture", host)
		for {
			line, err := c.ReadLine()
			if err != nil {
				return
			}
			switch {
			case strings.HasPrefix(line, "EHLO"):
				c.PrintfLine("250-%s", host)
				c.PrintfLine("250 AUTH PLAIN")
			case strings.HasPrefix(line, "AUTH"):
				if failAuth {
					c.PrintfLine("535 PRIVATE_SERVER_DETAIL")
					return
				}
				c.PrintfLine("235 accepted")
			case strings.HasPrefix(line, "MAIL FROM:"), strings.HasPrefix(line, "RCPT TO:"):
				c.PrintfLine("250 ok")
			case line == "DATA":
				c.PrintfLine("354 send data")
				data, err := c.ReadDotBytes()
				if err != nil {
					return
				}
				messages <- string(data)
				c.PrintfLine("250 queued")
			case line == "QUIT":
				c.PrintfLine("221 bye")
				return
			default:
				c.PrintfLine("500 unsupported")
			}
		}
	}()
	roots := x509.NewCertPool()
	parsed, _ := x509.ParseCertificate(der)
	roots.AddCert(parsed)
	origin, _ := actiontoken.NewOrigin("https://example.test")
	sender := &SMTP{origin: origin, settings: smtpSettings{Version: 1, Host: host, Port: 465, Username: "sender@example.test", From: "sender@example.test", Password: smtpFixtureSecret(t)}, tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, RootCAs: roots}, dial: func(ctx context.Context, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
	}}
	return sender, messages
}
func TestSMTPVerifiedTLSAndRedactedFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			sender, messages := smtpFixture(t, fail)
			secret, _ := actiontoken.Generate()
			link, _ := sender.origin.Link(actiontoken.ResetPassword, secret)
			err := sender.Send(context.Background(), Message{To: "recipient@example.test", Purpose: actiontoken.ResetPassword, Link: link})
			if fail {
				if err != ErrTransport || strings.Contains(err.Error(), "PRIVATE_SERVER_DETAIL") {
					t.Fatal("SMTP diagnostics leaked")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			data := <-messages
			parts := strings.SplitN(data, "\n\n", 2)
			if len(parts) != 2 {
				t.Fatal("invalid MIME")
			}
			decoded, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(parts[1], "\n", ""))
			if err != nil || !strings.Contains(string(decoded), link) {
				t.Fatal("fragment link missing")
			}
			if strings.Contains(parts[0], secret.Encoded()) || !strings.Contains(parts[0], "Content-Transfer-Encoding: base64") {
				t.Fatal("token in header")
			}
			for _, value := range []any{sender, Message{Link: link}} {
				encoded, _ := json.Marshal(value)
				if string(encoded) != "{}" || strings.Contains(fmt.Sprintf("%+v", value), secret.Encoded()) {
					t.Fatal("mail object leaked")
				}
			}
		})
	}
}
func TestSMTPRejectsUntrustedLinkAndUntrustedTLS(t *testing.T) {
	sender, _ := smtpFixture(t, false)
	secret, _ := actiontoken.Generate()
	link, _ := sender.origin.Link(actiontoken.ConfirmEmail, secret)
	if err := sender.Send(context.Background(), Message{To: "recipient@example.test", Purpose: actiontoken.ConfirmEmail, Link: strings.Replace(link, "example.test", "evil.test", 1)}); err != ErrTransport {
		t.Fatal("origin not pinned")
	}
	sender.tlsConfig.RootCAs = x509.NewCertPool()
	if err := sender.Send(context.Background(), Message{To: "recipient@example.test", Purpose: actiontoken.ConfirmEmail, Link: link}); err != ErrTransport {
		t.Fatal("untrusted TLS certificate accepted")
	}
}
func TestPrivateSMTPConfiguration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mail.json")
	fixture := map[string]any{"version": 1, "host": "smtp.yandex.ru", "port": 465, "username": "sender@example.test", "from": "sender@example.test", "password": smtpFixtureSecret(t), "public_origin": "https://example.test"}
	encodedFixture, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal("fixture encode failed")
	}
	valid := string(encodedFixture)
	for _, bad := range []string{strings.Replace(valid, `"version":1`, `"version":1,"version":1`, 1), strings.Replace(valid, `"port":465`, `"port":25`, 1), strings.Replace(valid, "smtp.yandex.ru", "https://smtp.example.test", 1), strings.Replace(valid, "https://example.test", "http://example.test", 1), valid + "{}"} {
		os.WriteFile(path, []byte(bad), 0600)
		if _, err := LoadSMTP(path); err != ErrConfiguration {
			t.Fatal("unsafe mail configuration accepted")
		}
	}
	os.WriteFile(path, []byte(valid), 0600)
	if _, err := LoadSMTP(path); err != nil {
		t.Fatal(err)
	}
	os.Chmod(path, 0644)
	if _, err := LoadSMTP(path); err != ErrConfiguration {
		t.Fatal("permissive secret accepted")
	}
	os.Chmod(path, 0600)
	link := filepath.Join(dir, "link.json")
	os.Symlink(path, link)
	if _, err := LoadSMTP(link); err != ErrConfiguration {
		t.Fatal("symlink accepted")
	}
}

func TestSMTPStartTLSCannotDowngradeToPlaintextAuth(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	auth := make(chan bool, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			auth <- false
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		c := textproto.NewConn(conn)
		c.PrintfLine("220 fixture")
		line, err := c.ReadLine()
		if err != nil || !strings.HasPrefix(line, "EHLO") {
			auth <- false
			return
		}
		c.PrintfLine("250 AUTH PLAIN")
		line, _ = c.ReadLine()
		auth <- strings.HasPrefix(line, "AUTH")
	}()
	origin, _ := actiontoken.NewOrigin("https://example.test")
	secret, _ := actiontoken.Generate()
	link, _ := origin.Link(actiontoken.ResetPassword, secret)
	sender := &SMTP{origin: origin, settings: smtpSettings{Host: "smtp.yandex.ru", Port: 587, Username: "sender@example.test", From: "sender@example.test", Password: smtpFixtureSecret(t)}, tlsConfig: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: "smtp.yandex.ru"}, dial: func(ctx context.Context, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", listener.Addr().String())
	}}
	if err = sender.Send(context.Background(), Message{To: "recipient@example.test", Purpose: actiontoken.ResetPassword, Link: link}); err != ErrTransport {
		t.Fatal("missing STARTTLS accepted")
	}
	if <-auth {
		t.Fatal("password sent before TLS")
	}
}

func TestConfiguredDomainSMTPUsesVerifiedHostname(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		t.Run(fmt.Sprint(mismatch), func(t *testing.T) {
			fixture, messages := smtpFixtureHost(t, false, "smtp.hosting.example")
			fixture.settings.PublicOrigin = "https://example.test"
			data, _ := json.Marshal(fixture.settings)
			path := filepath.Join(t.TempDir(), "mail.json")
			if os.WriteFile(path, data, 0600) != nil {
				t.Fatal("fixture write failed")
			}
			sender, err := LoadSMTP(path)
			if err != nil {
				t.Fatal("domain SMTP configuration rejected")
			}
			if sender.tlsConfig.ServerName != "smtp.hosting.example" || sender.tlsConfig.InsecureSkipVerify || sender.tlsConfig.MinVersion != tls.VersionTLS12 {
				t.Fatal("TLS endpoint not pinned")
			}
			sender.tlsConfig.RootCAs = fixture.tlsConfig.RootCAs
			sender.dial = func(ctx context.Context, address string) (net.Conn, error) {
				if address != "smtp.hosting.example:465" {
					t.Error("unexpected endpoint")
				}
				return fixture.dial(ctx, address)
			}
			if mismatch {
				sender.tlsConfig.ServerName = "other.hosting.example"
			}
			secret, _ := actiontoken.Generate()
			link, _ := sender.origin.Link(actiontoken.ConfirmEmail, secret)
			err = sender.Send(context.Background(), Message{To: "recipient@example.test", Purpose: actiontoken.ConfirmEmail, Link: link})
			if mismatch {
				if err != ErrTransport {
					t.Fatal("hostname mismatch accepted")
				}
			} else {
				if err != nil {
					t.Fatal("configured domain SMTP delivery failed")
				}
				if data := <-messages; !strings.Contains(data, "Content-Transfer-Encoding: base64") {
					t.Fatal("message not delivered")
				}
			}
		})
	}
}
func TestSMTPHostnameSyntax(t *testing.T) {
	for _, host := range []string{"smtp.yandex.ru", "smtp.hosting.example", "mail.xn--e1afmkfd.xn--p1ai"} {
		if !validSMTPHost(host) {
			t.Fatal("valid DNS endpoint rejected")
		}
	}
	for _, host := range []string{"", "localhost", "127.0.0.1", "::1", "https://mail.example", "mail.example:465", "mail.example/", "MAIL.example", "mail.example.", "mail..example", "-mail.example", "mail-.example", "mail.example\r\n", strings.Repeat("a", 64) + ".example"} {
		if validSMTPHost(host) {
			t.Fatal("noncanonical SMTP endpoint accepted")
		}
	}
}
