package updater

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"devcourse-finder/store"
)

// A provider outage must not consume the candidate limit, per-course retries or
// model budget. This anonymous API probe is independent of any particular course.
// The normal source reader still verifies every course after a successful probe.
func aiStepikAvailable(ctx context.Context, db *store.DB, client *http.Client) (bool, error) {
	var due, paused bool
	if err := db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM courses c
	 LEFT JOIN catalog_ai_sources a ON a.course_id=c.id
	 WHERE c.status='published' AND NOT c.demo AND c.source LIKE 'https://stepik.org/course/%'
	 AND (a.next_attempt_at IS NULL OR a.next_attempt_at<=now())),
	 EXISTS(SELECT 1 FROM catalog_ai_provider_health WHERE provider='stepik' AND next_probe_at>now() AND NOT available)`).Scan(&due, &paused); err != nil {
		return false, err
	}
	if paused || !due {
		return !paused, nil
	}
	probe, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	raw, _, err := fetch(probe, client, "https://stepik.org/api/courses?ids[]=58852")
	var payload struct {
		Courses []struct {
			ID int `json:"id"`
		} `json:"courses"`
	}
	available := err == nil && json.Unmarshal(raw, &payload) == nil && len(payload.Courses) == 1 && payload.Courses[0].ID == 58852
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	code := "available"
	if !available {
		code = "source_unavailable"
	}
	_, err = db.Pool.Exec(ctx, `INSERT INTO catalog_ai_provider_health(provider,available,code,checked_at,next_probe_at)
	 VALUES('stepik',$1,$2,now(),now()+interval '2 hours') ON CONFLICT(provider) DO UPDATE
	 SET available=$1,code=$2,checked_at=now(),next_probe_at=now()+interval '2 hours'`, available, code)
	return available, err
}

func aiProviderHealth(ctx context.Context, db *store.DB) ([]map[string]any, error) {
	rows, err := db.Pool.Query(ctx, `SELECT provider,available,code,checked_at,next_probe_at FROM catalog_ai_provider_health ORDER BY provider`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var provider, code string
		var available bool
		var checked, next time.Time
		if err := rows.Scan(&provider, &available, &code, &checked, &next); err != nil {
			return nil, err
		}
		items = append(items, map[string]any{"provider": provider, "available": available, "code": code, "checked_at": checked, "next_probe_at": next})
	}
	return items, rows.Err()
}
