package updater

import (
	"context"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
)

type aiProviderCoverage struct {
	Provider            string         `json:"provider"`
	Eligible            int            `json:"eligible_courses"`
	Mapped              int            `json:"mapped_courses"`
	Attempted           int            `json:"attempted_courses"`
	Verified            int            `json:"ever_verified_courses"`
	SourceFailures      int            `json:"source_failures"`
	NeverAttempted      int            `json:"never_attempted_courses"`
	Due                 int            `json:"due_courses"`
	MissingProfile      int            `json:"missing_profile_courses"`
	ProfileUnknown      map[string]int `json:"profile_unknown"`
	RecommendedProfile  map[string]int `json:"recommended_profile"`
	UnclassifiedProfile int            `json:"unclassified_profile_courses"`
}

// Coverage is operational state, not a claim of semantic completeness/accuracy.
// Validate canonical eligibility in Go just as the paid reader does.
func aiCoverage(ctx context.Context, db *store.DB) ([]aiProviderCoverage, error) {
	courses, e := db.Load(ctx)
	if e != nil {
		return nil, e
	}
	byCourse := map[string]catalog.Course{}
	for _, c := range courses {
		byCourse[c.ID] = c
	}
	rows, e := db.Pool.Query(ctx, `SELECT c.id,c.source,EXISTS(SELECT 1 FROM catalog_identities i WHERE (i.canonical_url=c.source OR (i.adapter IN ('yandex','otus') AND i.canonical_url||'/'=c.source)) AND i.course_id=c.id),s.attempted_at IS NOT NULL,s.verified_at IS NOT NULL,coalesce(s.code,''),(s.next_attempt_at IS NULL OR s.next_attempt_at<=now()) FROM courses c LEFT JOIN catalog_ai_sources s ON s.course_id=c.id WHERE c.status='published' AND NOT c.demo AND c.source LIKE ANY($1)`, aiSourcePatterns())
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	by := map[string]*aiProviderCoverage{}
	for _, id := range aiProviderIDs() {
		by[id] = &aiProviderCoverage{Provider: id, ProfileUnknown: map[string]int{"experience": 0, "goal": 0, "direction": 0}, RecommendedProfile: map[string]int{"experience": 0, "goal": 0, "direction": 0}}
	}
	for rows.Next() {
		var id, source, code string
		var mapped, attempted, verified, due bool
		if e = rows.Scan(&id, &source, &mapped, &attempted, &verified, &code, &due); e != nil {
			return nil, e
		}
		c, ok := aiCandidateURL(source)
		if !ok {
			continue
		}
		v := by[c.Adapter]
		v.Eligible++
		if aiMissingCount(byCourse[id]) > 0 {
			v.UnclassifiedProfile++
		}
		if !attempted {
			v.NeverAttempted++
		}
		if due {
			v.Due++
		}
		missing := false
		for field, unknown := range aiProfileMissing(byCourse[id]) {
			if unknown {
				v.ProfileUnknown[field]++
				missing = true
				if aiHasRecommendation(byCourse[id], field, time.Now()) {
					v.RecommendedProfile[field]++
				}
			}
		}
		if missing {
			v.MissingProfile++
		}
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
