package updater

import (
	"context"
	"devcourse-finder/store"
	"strings"
)

type aiProviderCoverage struct {
	Provider       string `json:"provider"`
	Eligible       int    `json:"eligible_courses"`
	Mapped         int    `json:"mapped_courses"`
	Attempted      int    `json:"attempted_courses"`
	Verified       int    `json:"ever_verified_courses"`
	SourceFailures int    `json:"source_failures"`
}

// Coverage is operational state, not a claim of semantic completeness/accuracy.
// Validate canonical eligibility in Go just as the paid reader does.
func aiCoverage(ctx context.Context, db *store.DB) ([]aiProviderCoverage, error) {
	rows, e := db.Pool.Query(ctx, `SELECT c.id,c.source,EXISTS(SELECT 1 FROM catalog_identities i WHERE i.canonical_url=c.source AND i.course_id=c.id),s.attempted_at IS NOT NULL,s.verified_at IS NOT NULL,coalesce(s.code,'') FROM courses c LEFT JOIN catalog_ai_sources s ON s.course_id=c.id WHERE c.status='published' AND NOT c.demo AND c.source LIKE ANY($1)`, aiSourcePatterns())
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	by := map[string]*aiProviderCoverage{}
	for _, id := range aiProviderIDs() {
		by[id] = &aiProviderCoverage{Provider: id}
	}
	for rows.Next() {
		var id, source, code string
		var mapped, attempted, verified bool
		if e = rows.Scan(&id, &source, &mapped, &attempted, &verified, &code); e != nil {
			return nil, e
		}
		c, ok := aiCandidateURL(source)
		if !ok {
			continue
		}
		v := by[c.Adapter]
		v.Eligible++
		if mapped {
			v.Mapped++
		}
		if attempted {
			v.Attempted++
		}
		if verified {
			v.Verified++
		}
		if strings.HasPrefix(code, "source_") {
			v.SourceFailures++
		}
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	out := []aiProviderCoverage{}
	for _, id := range aiProviderIDs() {
		out = append(out, *by[id])
	}
	return out, nil
}
