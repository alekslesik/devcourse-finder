package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
)

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	write(w, status, map[string]string{"error": msg})
}
func main() {
	if e := run(); e != nil {
		log.Print(e)
		os.Exit(1)
	}
}
func run() error {
	ctx := context.Background()
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "catalog" && len(args) >= 3 && args[1] == "validate" {
		f, e := os.Open(args[2])
		if e != nil {
			return e
		}
		defer f.Close()
		d, e := catalog.Decode(f)
		if e == nil {
			fmt.Printf("valid: %d courses\n", len(d.Courses))
		}
		return e
	}
	db, e := store.Open(ctx, os.Getenv("DATABASE_URL"))
	if e != nil {
		return e
	}
	defer db.Pool.Close()
	if len(args) > 0 {
		switch args[0] {
		case "migrate":
			return db.Migrate(ctx)
		case "catalog":
			if len(args) < 3 || args[1] != "import" {
				return fmt.Errorf("usage: catalog validate|import file [--dry-run]")
			}
			f, e := os.Open(args[2])
			if e != nil {
				return e
			}
			defer f.Close()
			d, e := catalog.Decode(f)
			if e != nil {
				return e
			}
			return db.Import(ctx, d, os.Getenv("USER"), slices.Contains(args, "--dry-run"))
		case "report":
			rows, e := db.Pool.Query(ctx, "SELECT day,kind,course_id,language,goal,count FROM daily_stats ORDER BY day DESC,kind")
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				v, e := rows.Values()
				if e != nil {
					return e
				}
				fmt.Println(v)
			}
			return rows.Err()
		case "cleanup":
			db.Cleanup(ctx)
			return nil
		default:
			return fmt.Errorf("unknown command")
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /health/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if db.Pool.Ping(ctx) != nil {
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
			log.Print("catalog load failed")
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
			if c.Slug == r.PathValue("slug") && c.Status != "draft" {
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
		ids := strings.Split(r.URL.Query().Get("offer_ids"), ",")
		if len(ids) > 3 || len(ids) == 0 || ids[0] == "" {
			fail(w, 400, "Выберите от 1 до 3 тарифов")
			return
		}
		cs, e := db.Load(r.Context())
		if e != nil {
			fail(w, 503, "Каталог недоступен")
			return
		}
		result := []any{}
		seen := map[string]bool{}
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			found := false
			for _, c := range cs {
				if c.Status != "published" {
					continue
				}
				for _, o := range c.Offers {
					if o.ID == id {
						result = append(result, catalog.Result{Course: c, Offer: o, Price: catalog.EffectivePrice(o, time.Now())})
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
			http.Error(w, "Каталог недоступен", http.StatusServiceUnavailable)
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
		http.Error(w, "Предложение недоступно", 404)
	})
	tokens := make(chan struct{}, 100)
	for i := 0; i < 100; i++ {
		tokens <- struct{}{}
	}
	go func() {
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			select {
			case tokens <- struct{}{}:
			default:
			}
		}
	}()
	mux.HandleFunc("POST /api/v1/events", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-tokens:
		default:
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
		if dec.Decode(&v) != nil || len(v.ID) < 8 || len(v.ID) > 80 || len(v.CourseID) > 100 || !slices.Contains([]string{"search", "empty", "view", "compare"}, v.Kind) || !slices.Contains([]string{"", "go", "python", "java", "javascript"}, v.Language) || !slices.Contains([]string{"", "try", "job", "switch", "deepen"}, v.Goal) || v.Total < 0 || v.Total > 1000000 {
			fail(w, 400, "Invalid event")
			return
		}
		if e := db.Event(r.Context(), v.ID, v.Kind, v.CourseID, v.Language, v.Goal, v.Total); e != nil {
			fail(w, 503, "Event unavailable")
			return
		}
		w.WriteHeader(204)
	})
	go func() {
		db.Cleanup(context.Background())
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for range t.C {
			db.Cleanup(context.Background())
		}
	}()
	server := &http.Server{Addr: ":8080", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		mux.ServeHTTP(w, r)
	}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	stop, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()
	go func() {
		<-stop.Done()
		c, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		server.Shutdown(c)
	}()
	log.Print("API listening on :8080")
	e = server.ListenAndServe()
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
func newID() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }
