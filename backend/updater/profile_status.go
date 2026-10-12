package updater

import (
	"context"
	"devcourse-finder/store"
	"encoding/json"
	"io"
	"time"
)

func profileStatus(ctx context.Context, db *store.DB, w io.Writer) error {
	rows, err := db.Pool.Query(ctx, `SELECT provider,count(*),
 count(*) FILTER (WHERE profile->'prerequisites' IS NOT NULL),
 count(*) FILTER (WHERE profile->'audience' IS NOT NULL),
 count(*) FILTER (WHERE profile->'goals' IS NOT NULL),
 count(*) FILTER (WHERE profile->'direction' IS NOT NULL),
 count(*) FILTER (WHERE profile->'target_level' IS NOT NULL),
 min((SELECT min((fact.value#>>'{evidence,checked_at}')::timestamptz) FROM jsonb_each(CASE WHEN jsonb_typeof(profile)='object' THEN profile ELSE '{}'::jsonb END) fact))
 FROM courses WHERE status='published' GROUP BY provider ORDER BY provider`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var provider string
		var courses, requirements, audience, goals, direction, target int
		var oldest *time.Time
		if err = rows.Scan(&provider, &courses, &requirements, &audience, &goals, &direction, &target, &oldest); err != nil {
			return err
		}
		if err = json.NewEncoder(w).Encode(map[string]any{"profile_coverage": true, "provider": provider, "courses": courses, "prerequisites": requirements, "audience": audience, "goals": goals, "direction": direction, "target_level": target, "oldest_fact_at": oldest}); err != nil {
			return err
		}
	}
	return rows.Err()
}
