// Package actionhttp exposes account actions only behind an authenticated proxy.
package actionhttp

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"devcourse-finder/auth/actiontoken"
	"devcourse-finder/auth/password"
	"devcourse-finder/internal/privatejson"
	"devcourse-finder/store"
)

const CookieName = "__Host-dcf-action-csrf"
const Prefix = "/api/v1/account/"

var ErrConfig = errors.New("invalid account HTTP configuration")

type Actions interface {
	Request(context.Context, string, actiontoken.Purpose) error
	Confirm(context.Context, string) error
	Reset(context.Context, string, string) error
}
type bucket struct {
	tokens float64
	last   time.Time
}

func (b *bucket) take(now time.Time, capacity, perSecond float64) bool {
	if b.last.IsZero() {
		b.tokens = capacity
		b.last = now
	}
	if now.After(b.last) {
		b.tokens = min(capacity, b.tokens+now.Sub(b.last).Seconds()*perSecond)
		b.last = now
	}
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

type Handler struct {
	service      Actions
	key          []byte
	origin       string
	mu           sync.Mutex
	global       bucket
	email        bucket
	ips          map[[32]byte]*bucket
	nonces       map[string]time.Time
	pruned       time.Time
	now          func() time.Time
	minimumDelay time.Duration
}

func New(service Actions, key []byte, rawOrigin string) (*Handler, error) {
	if service == nil || len(key) != 32 {
		return nil, ErrConfig
	}
	if _, err := actiontoken.NewOrigin(rawOrigin); err != nil {
		return nil, ErrConfig
	}
	u, _ := url.Parse(rawOrigin)
	return &Handler{service: service, key: append([]byte(nil), key...), origin: "https://" + u.Hostname(), ips: map[[32]byte]*bucket{}, nonces: map[string]time.Time{}, now: time.Now, minimumDelay: 350 * time.Millisecond}, nil
}
func (*Handler) String() string     { return "actionhttp.Handler(redacted)" }
func (h *Handler) GoString() string { return h.String() }
func signature(key []byte, r *http.Request, body []byte) []byte {
	cookie := ""
	if c, err := r.Cookie(CookieName); err == nil {
		cookie = c.Value
	}
	sum := sha256.Sum256(body)
	text := strings.Join([]string{"devcourse-finder/action-proxy/v1", r.Method, r.URL.Path, r.Header.Get("X-DCF-Action-Time"), r.Header.Get("X-DCF-Action-Nonce"), r.Header.Get("X-DCF-Action-IP"), r.Header.Get("Origin"), r.Header.Get("X-CSRF-Token"), cookie, hex.EncodeToString(sum[:])}, "\n")
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(text))
	return mac.Sum(nil)
}
func (h *Handler) authenticate(r *http.Request, body []byte) bool {
	timestamp := r.Header.Get("X-DCF-Action-Time")
	if len(timestamp) > 12 {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || strconv.FormatInt(seconds, 10) != timestamp {
		return false
	}
	now := h.now()
	delta := now.Unix() - seconds
	if delta < -30 || delta > 30 {
		return false
	}
	nonce := r.Header.Get("X-DCF-Action-Nonce")
	if len(nonce) != 32 {
		return false
	}
	if _, err = hex.DecodeString(nonce); err != nil {
		return false
	}
	received := r.Header.Get("X-DCF-Action-Signature")
	if len(received) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(received)
	if err != nil || !hmac.Equal(decoded, signature(h.key, r, body)) {
		return false
	}
	ip := r.Header.Get("X-DCF-Action-IP")
	if ip != "unknown" {
		address, err := netip.ParseAddr(ip)
		if err != nil {
			return false
		}
		address = address.Unmap()
		if address.Is6() {
			ip = netip.PrefixFrom(address, 64).Masked().String()
		} else {
			ip = address.String()
		}
	}
	mac := hmac.New(sha256.New, h.key)
	mac.Write([]byte("ip-limit/v1\x00" + ip))
	var id [32]byte
	copy(id[:], mac.Sum(nil))
	h.mu.Lock()
	defer h.mu.Unlock()
	if now.Sub(h.pruned) >= time.Minute {
		for n, expires := range h.nonces {
			if !expires.After(now) {
				delete(h.nonces, n)
			}
		}
		for key, b := range h.ips {
			if now.Sub(b.last) > 10*time.Minute {
				delete(h.ips, key)
			}
		}
		h.pruned = now
	}
	if _, exists := h.nonces[nonce]; exists {
		return false
	}
	if !h.global.take(now, 60, 1) {
		return false
	}
	b := h.ips[id]
	if b == nil {
		if len(h.ips) >= 4096 {
			return false
		}
		b = &bucket{}
		h.ips[id] = b
	}
	if !b.take(now, 12, .2) {
		return false
	}
	if strings.HasPrefix(strings.TrimPrefix(r.URL.Path, Prefix), "request-") && !h.email.take(now, 20, 1.0/3) {
		return false
	}
	h.nonces[nonce] = now.Add(time.Minute)
	return true
}
func reply(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message})
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "private, no-store, max-age=0")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Vary", "Origin, Cookie")
	started := time.Now()
	if r.URL.RawQuery != "" || r.URL.EscapedPath() != r.URL.Path {
		reply(w, 400, "invalid_request", "Некорректный запрос.")
		return
	}
	action := strings.TrimPrefix(r.URL.Path, Prefix)
	allowed := map[string]bool{"csrf": true, "request-confirmation": true, "request-reset": true, "confirm-email": true, "reset-password": true}
	if r.URL.Path != Prefix+action || !allowed[action] {
		reply(w, 404, "not_found", "Страница не найдена.")
		return
	}
	if action == "csrf" && r.Method != http.MethodGet || action != "csrf" && r.Method != http.MethodPost {
		reply(w, 405, "method_not_allowed", "Действие требует явного подтверждения.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil {
		reply(w, 413, "invalid_request", "Запрос слишком большой.")
		return
	}
	if !h.authenticate(r, body) {
		w.Header().Set("Retry-After", "5")
		reply(w, 429, "temporarily_limited", "Подождите немного и повторите попытку.")
		return
	}
	site := r.Header.Get("Sec-Fetch-Site")
	origin := r.Header.Get("Origin")
	if site != "" && site != "same-origin" || origin != "" && origin != h.origin || action != "csrf" && origin != h.origin {
		reply(w, 403, "invalid_origin", "Откройте форму на сайте сервиса.")
		return
	}
	if action == "csrf" {
		if len(body) != 0 {
			reply(w, 400, "invalid_request", "Некорректный запрос.")
			return
		}
		secret, err := actiontoken.Generate()
		if err != nil {
			reply(w, 503, "unavailable", "Попробуйте позже.")
			return
		}
		if c, err := r.Cookie(CookieName); err == nil {
			if existing, err := actiontoken.Parse(c.Value); err == nil {
				secret = existing
			}
		}
		http.SetCookie(w, &http.Cookie{Name: CookieName, Value: secret.Encoded(), Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{"token": secret.Encoded()})
		return
	}
	cookie, err := r.Cookie(CookieName)
	csrf := r.Header.Get("X-CSRF-Token")
	if err != nil || len(csrf) != 46 || len(cookie.Value) != 46 || subtle.ConstantTimeCompare([]byte(csrf), []byte(cookie.Value)) != 1 {
		reply(w, 403, "invalid_csrf", "Обновите форму и повторите попытку.")
		return
	}
	if _, err := actiontoken.Parse(csrf); err != nil {
		reply(w, 403, "invalid_csrf", "Обновите форму и повторите попытку.")
		return
	}
	content, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || content != "application/json" {
		reply(w, 415, "invalid_request", "Некорректный запрос.")
		return
	}
	var input struct {
		Email    string `json:"email"`
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	fields := []string{"token"}
	if strings.HasPrefix(action, "request-") {
		fields = []string{"email"}
	} else if action == "reset-password" {
		fields = []string{"token", "password"}
	}
	if privatejson.Decode(body, &input, fields...) != nil {
		reply(w, 400, "invalid_request", "Проверьте данные формы.")
		return
	}

	if strings.HasPrefix(action, "request-") {
		if input.Token != "" || input.Password != "" || len(input.Email) > 1024 {
			reply(w, 400, "invalid_request", "Проверьте данные формы.")
			return
		}
		purpose := actiontoken.ConfirmEmail
		if action == "request-reset" {
			purpose = actiontoken.ResetPassword
		}
		_ = h.service.Request(r.Context(), input.Email, purpose)
		// Uniform response and minimum delay apply to ineligible, unknown, throttled
		// and temporarily failed requests alike; no email address is echoed or logged.
		jitter, _ := actiontoken.Generate()
		digest, _ := jitter.Digest(actiontoken.ConfirmEmail)
		delay := h.minimumDelay + time.Duration(digest[0]%76)*time.Millisecond - time.Since(started)
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-timer.C:
			case <-r.Context().Done():
				return
			}
		}
		reply(w, 202, "accepted", "Если адрес подходит для этого действия, письмо придёт в ближайшее время.")
		return
	}
	if input.Email != "" || action == "confirm-email" && input.Password != "" {
		reply(w, 400, "invalid_request", "Проверьте данные формы.")
		return
	}
	if action == "confirm-email" {
		err = h.service.Confirm(r.Context(), input.Token)
	} else {
		err = h.service.Reset(r.Context(), input.Token, input.Password)
	}
	switch {
	case err == nil:
		reply(w, 200, "completed", "Действие выполнено.")
	case errors.Is(err, store.ErrActionToken):
		reply(w, 400, "invalid_link", "Ссылка недействительна или устарела. Запросите новую.")
	case errors.Is(err, password.ErrCommonPassword):
		reply(w, 422, "password_common", "Выберите менее распространённый пароль.")
	case errors.Is(err, password.ErrPasswordInput):
		reply(w, 422, "password_policy", "Нужно не менее 15 символов и не более 1024 байт.")
	case errors.Is(err, password.ErrBusy):
		w.Header().Set("Retry-After", "2")
		reply(w, 429, "temporarily_limited", "Подождите немного и повторите попытку.")
	default:
		reply(w, 503, "unavailable", "Сервис временно недоступен. Попробуйте позже.")
	}
}
