package updater

import (
	"context"
	"errors"
	"time"

	"devcourse-finder/store"
	"encoding/json"
	"io"
)

func ValidateWorker(worker string) error {
	switch worker {
	case "", "api", "pages", "publisher":
		return nil
	default:
		return errors.New("CATALOG_WORKER must be api, pages or publisher (empty selects legacy)")
	}
}
func workerName(worker string) string {
	if worker == "" {
		return "legacy"
	}
	return worker
}
func sourceAdapter(adapter string) string {
	if adapter == "schema-course" {
		return "codebasics"
	}
	return adapter
}
func owner(adapter string) string {
	if adapter == "stepik" || adapter == "yandex" {
		return "api"
	}
	return "pages"
}
func (c Config) ForWorker(worker string) (Config, error) {
	if err := ValidateWorker(worker); err != nil {
		return Config{}, err
	}
	if worker == "" || worker == "publisher" {
		return c, c.Validate()
	}
	filtered := c
	filtered.Sources = nil
	filtered.Discovery = nil
	for _, source := range c.Sources {
		if owner(sourceAdapter(source.Adapter)) == worker {
			filtered.Sources = append(filtered.Sources, source)
		}
	}
	for _, feed := range c.Discovery {
		if owner(feed.Adapter) == worker {
			filtered.Discovery = append(filtered.Discovery, feed)
		}
	}
	return filtered, filtered.Validate()
}
func heartbeatWorker(ctx context.Context, db *store.DB, worker string) error {
	key := "updater_heartbeat"
	if worker != "" {
		key += "_" + worker
	}
	_, err := db.Pool.Exec(ctx, `INSERT INTO settings(key,value) VALUES($1,to_jsonb(now())) ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value`, key)
	return err
}
func HealthWorker(ctx context.Context, db *store.DB, worker string) error {
	if err := ValidateWorker(worker); err != nil {
		return err
	}
	key := "updater_heartbeat"
	if worker != "" {
		key += "_" + worker
	}
	var healthy bool
	if err := db.Pool.QueryRow(ctx, `SELECT (value #>> '{}')::timestamptz > now()-interval '90 seconds' FROM settings WHERE key=$1`, key).Scan(&healthy); err != nil {
		return err
	}
	if !healthy {
		return errors.New("worker heartbeat is stale")
	}
	return nil
}
func observationStatus(ctx context.Context, db *store.DB, w io.Writer) error {
	var pending, processed int
	var oldest, last *time.Time
	err := db.Pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE processed_at IS NULL),count(*) FILTER(WHERE processed_at IS NOT NULL),min(created_at) FILTER(WHERE processed_at IS NULL),max(processed_at) FROM catalog_observations`).Scan(&pending, &processed, &oldest, &last)
	if err != nil {
		return err
	}
	if err = json.NewEncoder(w).Encode(map[string]any{"publication_queue": true, "pending": pending, "retained_processed": processed, "oldest_pending_at": oldest, "last_processed_at": last}); err != nil {
		return err
	}
	for _, role := range []string{"api", "pages", "publisher"} {
		var beat *time.Time
		if err = db.Pool.QueryRow(ctx, `SELECT max((value #>> '{}')::timestamptz) FROM settings WHERE key=$1`, "updater_heartbeat_"+role).Scan(&beat); err != nil {
			return err
		}
		alive := beat != nil && time.Since(*beat) < 90*time.Second
		if err = json.NewEncoder(w).Encode(map[string]any{"worker": role, "heartbeat_at": beat, "alive": alive}); err != nil {
			return err
		}
	}
	return nil
}
