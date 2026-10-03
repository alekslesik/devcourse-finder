package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"devcourse-finder/catalog"
)

type catalogRepository interface {
	Load(context.Context) ([]catalog.Course, error)
	Domains(context.Context) []string
	Event(context.Context, string, string, string, string, string, int) error
	Ping(context.Context) error
}

// newHandler is shared by the real server and HTTP tests.
func newHandler(db catalogRepository) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if db.Ping(ctx) != nil {
			fail(w, 503, "database unavailable")
			return
		}
		write(w, 200, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/v1/catalog/options", func(w http.ResponseWriter, r *http.Request) {
		write(w, 200, map[string]any{"languages": []string{"go", "python", "java", "javascript"}, "directions": []string{"basics", "backend", "frontend", "fullstack", "automation"}})
	})
	mux.HandleFunc("GET /api/v1/courses", func(w http.ResponseWriter, r *http.Request) {
		f, e := catalog.Parse(r.URL.Query())
		if e != nil {
			fail(w, 400, e.Error())
			return
		}
		cs, e := db.Load(r.Context())
		if e != nil {
			slog.Error("catalog load failed", "request_id", w.Header().Get("X-Request-ID"))
			fail(w, 503, "Каталог временно недоступен")
			return
		}
		now := time.Now()
		items := catalog.Search(cs, f, now)
		total := len(items)
		start := (f.Page - 1) * f.PageSize
		if start > total {
			start = total
		}
		end := start + f.PageSize
		if end > total {
			end = total
		}
		write(w, 200, map[string]any{"items": items[start:end], "total": total, "page": f.Page, "page_size": f.PageSize, "data_as_of": now, "applied_filters": r.URL.Query()})
	})
	mux.HandleFunc("GET /api/v1/courses/{slug}", func(w http.ResponseWriter, r *http.Request) {
		cs, e := db.Load(r.Context())
		if e != nil {
			fail(w, 503, "Каталог недоступен")
			return
		}
		for _, c := range cs {
			if c.Slug == r.PathValue("slug") && c.Status == "published" {
				offers := []catalog.Result{}
				for _, o := range c.Offers {
					offers = append(offers, catalog.Result{Course: c, Offer: o, Price: catalog.EffectivePrice(o, time.Now())})
				}
				write(w, 200, map[string]any{"course": c, "offers": offers})
				return
			}
		}
		fail(w, 404, "Программа не найдена")
	})
	mux.HandleFunc("GET /api/v1/compare", func(w http.ResponseWriter, r *http.Request) {
		ids := []string{}
		for _, id := range strings.Split(r.URL.Query().Get("offer_ids"), ",") {
			if id == "" {
				fail(w, 400, "Выберите от 1 до 3 тарифов")
				return
			}
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		if len(ids) > 3 {
			fail(w, 400, "Выберите от 1 до 3 тарифов")
			return
		}
		cs, e := db.Load(r.Context())
		if e != nil {
			fail(w, 503, "Каталог недоступен")
			return
		}
		result := []any{}
		for _, id := range ids {
			found := false
			for _, c := range cs {
				if c.Status != "published" {
					continue
				}
				for _, o := range c.Offers {
					if o.ID == id {
						if o.Enrollment == "closed" {
							result = append(result, map[string]any{"id": id, "unavailable": true, "reason": "closed"})
						} else {
							result = append(result, catalog.Result{Course: c, Offer: o, Price: catalog.EffectivePrice(o, time.Now())})
						}
						found = true
					}
				}
			}
			if !found {
				result = append(result, map[string]any{"id": id, "unavailable": true})
			}
		}
		write(w, 200, result)
	})
	mux.HandleFunc("GET /out/{id}", func(w http.ResponseWriter, r *http.Request) {
		cs, e := db.Load(r.Context())
		if e != nil {
			fail(w, http.StatusServiceUnavailable, "Каталог недоступен")
			return
		}
		for _, c := range cs {
			if c.Status != "published" {
				continue
			}
			for _, o := range c.Offers {
				if o.ID == r.PathValue("id") && catalog.SafeURL(o.URL, db.Domains(r.Context())) {
					ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
					db.Event(ctx, newID(), "outbound", c.ID, c.Language, "", 0)
					cancel()
					w.Header().Set("Cache-Control", "no-store")
					http.Redirect(w, r, o.URL, http.StatusFound)
					return
				}
			}
		}
		fail(w, 404, "Предложение недоступно")
	})
	limiter := eventLimiter{tokens: 100, last: time.Now()}
	mux.HandleFunc("POST /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		if !limiter.allow(time.Now()) {
			fail(w, 429, "Too many events")
			return
		}
		var v struct {
			ID       string `json:"id"`
			Kind     string `json:"kind"`
			CourseID string `json:"course_id"`
			Language string `json:"language"`
			Goal     string `json:"goal"`
			Total    int    `json:"total"`
		}
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
		dec.DisallowUnknownFields()
		if dec.Decode(&v) != nil || dec.Decode(&struct{}{}) != io.EOF || len(v.ID) < 8 || len(v.ID) > 80 || len(v.CourseID) > 100 || !slices.Contains([]string{"search", "empty", "view", "compare"}, v.Kind) || !slices.Contains([]string{"", "go", "python", "java", "javascript"}, v.Language) || !slices.Contains([]string{"", "try", "job", "switch", "deepen"}, v.Goal) || v.Total < 0 || v.Total > 1000000 {
			fail(w, 400, "Invalid event")
			return
		}
		if e := db.Event(r.Context(), v.ID, v.Kind, v.CourseID, v.Language, v.Goal, v.Total); e != nil {
			fail(w, 503, "Event unavailable")
			return
		}
		w.WriteHeader(204)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-ID", newID())
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		started := time.Now()
		if _, pattern := mux.Handler(r); pattern == "" {
			mux.ServeHTTP(&routingErrorWriter{ResponseWriter: w}, r)
		} else {
			mux.ServeHTTP(w, r)
		}
		slog.Info("request completed", "request_id", w.Header().Get("X-Request-ID"), "method", r.Method, "duration_ms", time.Since(started).Milliseconds())
	})
}

// Replenish at 100 events/second with a burst of 100 without a background worker.
type eventLimiter struct {
	mu     sync.Mutex
	tokens float64
	last   time.Time
}

func (l *eventLimiter) allow(now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if now.After(l.last) {
		l.tokens = min(100, l.tokens+now.Sub(l.last).Seconds()*100)
		l.last = now
	}
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}

type routingErrorWriter struct {
	http.ResponseWriter
	failed bool
}

func (w *routingErrorWriter) WriteHeader(status int) {
	if status >= 400 {
		w.failed = true
		w.Header().Del("Content-Length")
		fail(w.ResponseWriter, status, http.StatusText(status))
		return
	}
	w.ResponseWriter.WriteHeader(status)
}
func (w *routingErrorWriter) Write(body []byte) (int, error) {
	if w.failed {
		return len(body), nil
	}
	return w.ResponseWriter.Write(body)
}
