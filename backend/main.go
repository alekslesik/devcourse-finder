package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"devcourse-finder/updater"
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
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)).With("command", commandName(os.Args[1:]), "run_id", newID()))
	if e := run(); e != nil {
		slog.LogAttrs(context.Background(), slog.LevelError, "command failed", commandErrorAttrs(e)...)
		os.Exit(1)
	}
}
func run() error {
	ctx := context.Background()
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "catalog" && len(args) >= 3 && args[1] == "validate" {
		f, e := os.Open(args[2])
		if e != nil {
			return commandFailure("read_catalog", "catalog_file_unavailable", "Cannot read catalog file", e)
		}
		defer f.Close()
		d, e := catalog.Decode(f)
		if e == nil {
			fmt.Printf("valid: %d courses\n", len(d.Courses))
		}
		if e != nil {
			return commandFailure("validate_catalog", "invalid_catalog", "Catalog validation failed", e)
		}
		return nil
	}
	if os.Getenv("APP_ENV") == "production" {
		password := os.Getenv("PGPASSWORD")
		if raw := os.Getenv("DATABASE_URL"); raw != "" {
			connection, err := url.Parse(raw)
			if err != nil {
				return commandFailure("database_config", "invalid_database_configuration", "Invalid database configuration", err)
			}
			if connection.User != nil {
				password, _ = connection.User.Password()
			}
		}
		if password == "" || password == "devcourse-local" {
			return commandFailure("database_config", "invalid_database_configuration", "Production requires a non-demo database password", nil)
		}
	}
	db, e := store.Open(ctx, os.Getenv("DATABASE_URL"))
	if e != nil {
		return commandFailure("database_config", "invalid_database_configuration", "Invalid database configuration", e)
	}
	defer db.Pool.Close()
	if len(args) > 0 {
		switch args[0] {
		case "catalog-update":
			if len(args) != 2 {
				return commandFailure("arguments", "invalid_arguments", "Usage: catalog-update serve|once|status|health", nil)
			}
			stop, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
			defer cancel()
			worker := os.Getenv("CATALOG_WORKER")
			if err := updater.ValidateWorker(worker); err != nil {
				return commandFailure("updater_config", "invalid_worker", "Invalid catalog worker role", err)
			}
			if args[1] == "health" {
				bounded, done := context.WithTimeout(stop, 5*time.Second)
				defer done()
				if err := updater.HealthWorker(bounded, db, worker); err != nil {
					return commandFailure("updater_health", "updater_unhealthy", "Catalog updater is not healthy", err)
				}
				return nil
			}
			if args[1] == "status" {
				return updater.Status(stop, db, os.Stdout)
			}
			if args[1] != "serve" && args[1] != "once" {
				return commandFailure("arguments", "invalid_arguments", "Usage: catalog-update serve|once|status|health", nil)
			}
			configFile := os.Getenv("CATALOG_UPDATE_SOURCES")
			if configFile == "" {
				configFile = "/data/updater-sources.json"
			}
			templateFile := os.Getenv("CATALOG_UPDATE_TEMPLATES")
			if templateFile == "" {
				templateFile = "/data/real-catalog.json"
			}
			config, err := updater.LoadConfig(configFile, templateFile)
			if err != nil {
				return commandFailure("updater_config", "invalid_updater_configuration", "Cannot validate catalog updater configuration", err)
			}
			service := updater.Service{DB: db, Config: config, Worker: worker}
			if args[1] == "serve" {
				err = service.Serve(stop)
			} else {
				bounded, done := context.WithTimeout(stop, 10*time.Minute)
				defer done()
				var result updater.Result
				result, err = service.Run(bounded)
				// The background publisher may own the lock while manual collection finishes.
				// Wait only for this expected contention, within the existing command deadline.
				for worker == "publisher" && errors.Is(err, updater.ErrBusy) {
					select {
					case <-bounded.Done():
						err = bounded.Err()
					case <-time.After(time.Second):
						result, err = service.Run(bounded)
					}
				}
				if err == nil && result.Status == "failed" {
					err = fmt.Errorf("all catalog sources failed verification")
				}
			}
			if err != nil {
				return commandFailure("catalog_update", "catalog_update_failed", "Catalog collection did not complete", err)
			}
			return nil
		case "migrate":
			if err := db.Migrate(ctx); err != nil {
				return commandFailure("migrate", "migration_failed", "Database migration failed", err)
			}
			return nil
		case "catalog":
			if len(args) < 3 || args[1] != "import" {
				return commandFailure("arguments", "invalid_arguments", "Usage: catalog validate|import file [--dry-run]", nil)
			}
			operator, e := catalogOperator(os.Getenv("CATALOG_OPERATOR"))
			if e != nil {
				return commandFailure("operator", "invalid_catalog_operator", "CATALOG_OPERATOR must contain 1 to 100 characters", e)
			}
			f, e := os.Open(args[2])
			if e != nil {
				return commandFailure("read_catalog", "catalog_file_unavailable", "Cannot read catalog file", e)
			}
			defer f.Close()
			d, e := catalog.Decode(f)
			if e != nil {
				return commandFailure("validate_catalog", "invalid_catalog", "Catalog validation failed", e)
			}
			dry := slices.Contains(args, "--dry-run")
			started := time.Now()
			if err := db.Import(ctx, d, operator, dry); err != nil {
				return commandFailure("publish_catalog", "catalog_import_failed", "Catalog import did not complete", err)
			}
			slog.Info("catalog import completed", "courses", len(d.Courses), "dry_run", dry, "duration_ms", time.Since(started).Milliseconds())
			return nil
		case "report":
			rows, e := db.Pool.Query(ctx, "SELECT day,kind,course_id,language,goal,count FROM daily_stats ORDER BY day DESC,kind")
			if e != nil {
				return commandFailure("report", "report_failed", "Cannot read analytics report", e)
			}
			defer rows.Close()
			for rows.Next() {
				v, e := rows.Values()
				if e != nil {
					return commandFailure("report", "report_failed", "Cannot read analytics report", e)
				}
				fmt.Println(v)
			}
			if err := rows.Err(); err != nil {
				return commandFailure("report", "report_failed", "Cannot read analytics report", err)
			}
			return nil
		case "cleanup":
			db.Cleanup(ctx)
			return nil
		default:
			return commandFailure("arguments", "invalid_arguments", "Unknown command", nil)
		}
	}
	stop, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
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
	return commandFailure("serve", "server_failed", "HTTP server failed", e)
}
func newID() string { b := make([]byte, 16); rand.Read(b); return hex.EncodeToString(b) }
