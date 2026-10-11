package updater

import (
	"context"
	"devcourse-finder/store"
	"encoding/json"
	"io"
	"time"
)

func learningStatus(ctx context.Context, db *store.DB, w io.Writer) error {
	rows, err := db.Pool.Query(ctx, `SELECT c.provider,count(*),
 count(*) FILTER(WHERE o.learning_conditions->'duration' IS NOT NULL),
 count(*) FILTER(WHERE o.learning_conditions->'weekly_workload' IS NOT NULL),
 count(*) FILTER(WHERE o.learning_conditions->'review' IS NOT NULL),
 count(*) FILTER(WHERE o.learning_conditions->'mentor' IS NOT NULL),
 count(*) FILTER(WHERE o.learning_conditions->'community' IS NOT NULL),
 min((SELECT min((fact.value#>>'{evidence,checked_at}')::timestamptz) FROM jsonb_each(CASE WHEN jsonb_typeof(o.learning_conditions)='object' THEN o.learning_conditions ELSE '{}'::jsonb END) fact))
 FROM offers o JOIN courses c ON c.id=o.course_id WHERE c.status='published' GROUP BY c.provider ORDER BY c.provider`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var provider string
		var offers, duration, workload, review, mentor, community int
		var oldest *time.Time
		if err = rows.Scan(&provider, &offers, &duration, &workload, &review, &mentor, &community, &oldest); err != nil {
			return err
		}
		if err = json.NewEncoder(w).Encode(map[string]any{"learning_conditions_coverage": true, "provider": provider, "offers": offers, "duration": duration, "weekly_workload": workload, "review": review, "mentor": mentor, "community": community, "oldest_fact_at": oldest}); err != nil {
			return err
		}
	}
	return rows.Err()
}
