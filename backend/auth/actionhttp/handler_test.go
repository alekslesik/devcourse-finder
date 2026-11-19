package actionhttp

import (
	"bytes"
	"context"
	"crypto/rand"
	"devcourse-finder/auth/actiontoken"
	"devcourse-finder/store"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

type fakeActions struct {
	requests int
	confirms int
	resets   int
	fail     bool
}

func (s *fakeActions) Request(context.Context, string, actiontoken.Purpose) error {
	s.requests++
	if s.fail {
		return store.ErrUserStorage
	}
	return nil
}
func (s *fakeActions) Confirm(context.Context, string) error       { s.confirms++; return nil }
func (s *fakeActions) Reset(context.Context, string, string) error { s.resets++; return nil }
func fixture(t *testing.T) (*Handler, *fakeActions) {
	t.Helper()
	service := &fakeActions{}
	h, err := New(service, bytes.Repeat([]byte{7}, 32), "https://example.test")
	if err != nil {
		t.Fatal(err)
	}
	h.minimumDelay = 0
	return h, service
}
func signed(h *Handler, method, action, body, csrf string) *http.Request {
	r := httptest.NewRequest(method, Prefix+action, strings.NewReader(body))
	r.Header.Set("Origin", "https://example.test")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	if csrf != "" {
		r.AddCookie(&http.Cookie{Name: CookieName, Value: csrf})
		r.Header.Set("X-CSRF-Token", csrf)
	}
	r.Header.Set("X-DCF-Action-Time", strconv.FormatInt(h.now().Unix(), 10))
	var nonce [16]byte
	rand.Read(nonce[:])
	r.Header.Set("X-DCF-Action-Nonce", hex.EncodeToString(nonce[:]))
	r.Header.Set("X-DCF-Action-IP", "198.51.100.10")
	r.Header.Set("X-DCF-Action-Signature", hex.EncodeToString(signature(h.key, r, []byte(body))))
	return r
}
func csrfCookie(t *testing.T, h *Handler) string {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signed(h, "GET", "csrf", "", ""))
	if w.Code != 200 {
		t.Fatal("CSRF unavailable")
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != CookieName || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Domain != "" || cookies[0].Path != "/" {
		t.Fatal("unsafe CSRF cookie")
	}
	var body map[string]string
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["token"] != cookies[0].Value {
		t.Fatal("CSRF mismatch")
	}
	return body["token"]
}
func TestExplicitActionAndProxyIntegrity(t *testing.T) {
	h, s := fixture(t)
	csrf := csrfCookie(t, h)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, signed(h, "GET", "confirm-email", "", csrf))
	if w.Code != 405 || s.confirms != 0 {
		t.Fatal("GET mutated account")
	}
	r := signed(h, "POST", "confirm-email", `{"token":"fixture"}`, csrf)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || s.confirms != 1 {
		t.Fatal("explicit action failed")
	}
	replay := r.Clone(context.Background())
	replay.Body = ioBody(`{"token":"fixture"}`)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, replay)
	if w.Code != 429 || s.confirms != 1 {
		t.Fatal("proxy replay accepted")
	}
	tampered := signed(h, "POST", "reset-password", `{"token":"fixture","password":"synthetic value"}`, csrf)
	tampered.Body = ioBody(`{"token":"changed","password":"synthetic value"}`)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, tampered)
	if w.Code != 429 || s.resets != 0 {
		t.Fatal("body signature bypass")
	}
}
func ioBody(value string) io.ReadCloser { return io.NopCloser(strings.NewReader(value)) }
func TestOriginAndCSRFEnforcement(t *testing.T) {
	for _, kind := range []string{"origin", "site", "csrf", "proxy", "query"} {
		t.Run(kind, func(t *testing.T) {
			h, s := fixture(t)
			csrf := csrfCookie(t, h)
			body := `{"token":"fixture"}`
			r := signed(h, "POST", "confirm-email", body, csrf)
			switch kind {
			case "origin":
				r.Header.Set("Origin", "https://evil.test")
			case "site":
				r.Header.Set("Sec-Fetch-Site", "cross-site")
			case "csrf":
				r.Header.Set("X-CSRF-Token", "")
			case "proxy":
				r.Header.Set("X-DCF-Action-Signature", "")
			case "query":
				r.URL.RawQuery = "token=must-not-consume"
			}
			if kind != "proxy" {
				r.Header.Set("X-DCF-Action-Signature", hex.EncodeToString(signature(h.key, r, []byte(body))))
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code < 400 || s.confirms != 0 {
				t.Fatal("cross-site or unsigned action accepted")
			}
		})
	}
}
func TestUniformRequestsAndIPBudget(t *testing.T) {
	h, s := fixture(t)
	csrf := csrfCookie(t, h)
	var accepted string
	for _, fail := range []bool{false, true} {
		s.fail = fail
		w := httptest.NewRecorder()
		h.ServeHTTP(w, signed(h, "POST", "request-reset", `{"email":"missing@example.test"}`, csrf))
		if w.Code != 202 {
			t.Fatal("response reveals eligibility")
		}
		if accepted != "" && w.Body.String() != accepted {
			t.Fatal("response reveals backend error")
		}
		accepted = w.Body.String()
		if strings.Contains(accepted, "missing@") {
			t.Fatal("email echoed")
		}
	}
	limited := false
	for i := 0; i < 15; i++ {
		r := signed(h, "GET", "csrf", "", "")
		r.Header.Set("X-Forwarded-For", strconv.Itoa(i)+".1.1.1")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code == 429 {
			limited = true
			break
		}
	}
	if !limited {
		t.Fatal("forwarded spoof bypassed authenticated-IP limit")
	}
}

func TestNodeAndGoProxySignatureAgree(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for cross-language gateway vector")
	}
	h, _ := fixture(t)
	var key [32]byte
	rand.Read(key[:])
	secret, _ := actiontoken.Generate()
	csrf := secret.Encoded()
	body := []byte(`{"token":"synthetic","password":"a synthetic private value"}`)
	r := signed(h, "POST", "reset-password", string(body), csrf)
	metadata := map[string]string{"method": r.Method, "path": r.URL.Path, "time": r.Header.Get("X-DCF-Action-Time"), "nonce": r.Header.Get("X-DCF-Action-Nonce"), "ip": r.Header.Get("X-DCF-Action-IP"), "origin": r.Header.Get("Origin"), "csrf": csrf, "cookie": csrf}
	data, _ := json.Marshal(map[string]any{"key": base64.StdEncoding.EncodeToString(key[:]), "metadata": metadata, "body": base64.StdEncoding.EncodeToString(body)})
	script := `import {readFileSync} from 'node:fs';import {actionSignature} from '../../../frontend/lib/account-proxy.mjs';const x=JSON.parse(readFileSync(0,'utf8'));process.stdout.write(actionSignature(Buffer.from(x.key,'base64'),x.metadata,Buffer.from(x.body,'base64')));`
	command := exec.Command(node, "--input-type=module", "-e", script)
	command.Stdin = bytes.NewReader(data)
	output, err := command.Output()
	if err != nil {
		t.Fatal("Node gateway vector unavailable")
	}
	if string(output) != hex.EncodeToString(signature(key[:], r, body)) {
		t.Fatal("Node/Go proxy protocol mismatch")
	}
}
