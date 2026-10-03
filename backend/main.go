package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

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
	code := map[int]string{400: "invalid_request", 404: "not_found", 405: "method_not_allowed", 429: "rate_limited", 503: "unavailable"}[status]
	if code == "" {
		code = "internal_error"
	}
	write(w, status, map[string]string{"error": msg, "code": code, "request_id": w.Header().Get("X-Request-ID")})
}
func catalogOperator(raw string) (string, error) {
	operator := strings.TrimSpace(raw)
	if !utf8.ValidString(operator) || utf8.RuneCountInString(operator) < 1 || utf8.RuneCountInString(operator) > 100 {
		return "", fmt.Errorf("CATALOG_OPERATOR must contain 1 to 100 characters")
	}
	return operator, nil
}
func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	if e := run(); e != nil {
		slog.Error("command failed", "error", e.Error())
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
	if os.Getenv("APP_ENV") == "production" {
		password := os.Getenv("PGPASSWORD")
		if raw := os.Getenv("DATABASE_URL"); raw != "" {
			connection, err := url.Parse(raw)
			if err != nil {
				return fmt.Errorf("invalid database configuration")
			}
			if connection.User != nil {
				password, _ = connection.User.Password()
			}
		}
		if password == "" || password == "devcourse-local" {
			return fmt.Errorf("production requires a database password different from the demo default")
		}
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
			operator, e := catalogOperator(os.Getenv("CATALOG_OPERATOR"))
			if e != nil {
				return e
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
			return db.Import(ctx, d, operator, slices.Contains(args, "--dry-run"))
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
	stop, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()
	go func() {
		db.Cleanup(stop)
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			select {
			case <-stop.Done():
				return
			case <-t.C:
				db.Cleanup(stop)
			}
		}
	}()
	server := &http.Server{Addr: ":8080", Handler: newHandler(store.NewCached(db)), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-stop.Done()
		c, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		server.Shutdown(c)
	}()
	slog.Info("API listening", "port", 8080)
	e = server.ListenAndServe()
	if e == http.ErrServerClosed {
		return nil
	}
	return e
}
func newID() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }
