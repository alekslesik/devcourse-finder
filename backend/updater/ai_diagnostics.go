package updater

import (
	"context"
	"devcourse-finder/store"
)

// Full source-state counts; the following per-course status lines are only the
// most recent forty records and must not be used as catalog-wide totals.
func aiSourceCodes(ctx context.Context, db *store.DB) (map[string]int, error) {
	rows, err := db.Pool.Query(ctx, "SELECT code,count(*) FROM catalog_ai_sources GROUP BY code")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var code string
		var count int
		if err := rows.Scan(&code, &count); err != nil {
			return nil, err
		}
		out[code] = count
	}
	return out, rows.Err()
}
