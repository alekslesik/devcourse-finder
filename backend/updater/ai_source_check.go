package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
)

// Explicit, bounded published-course selection. Never accept model URLs, broad
// provider patterns, duplicates or an accidentally empty manual selection.
func ParseAICourseIDs(raw string) ([]string, error) {
	ids := strings.Split(raw, ",")
	if len(ids) > 8 {
		return nil, ErrAI
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,99}$`).MatchString(id) || seen[id] {
			return nil, ErrAI
		}
		seen[id] = true
	}
	return ids, nil
}

type aiSourceCheckResult struct {
	CourseID        string `json:"course_id"`
	Code            string `json:"code"`
	CourseDocuments int    `json:"course_documents"`
	OfferDocuments  int    `json:"offer_documents"`
	TextBytes       int    `json:"text_bytes"`
	Partial         bool   `json:"partial"`
}

// AISourceCheck reads current official sources using the production readers and
// saved mappings. It never invokes models or changes queue/cache/freshness/budget.
// Partial course readiness is deliberately distinct from tariff readiness.
func AISourceCheck(ctx context.Context, db *store.DB, client *http.Client, rawIDs string, w io.Writer) error {
	ids, err := ParseAICourseIDs(rawIDs)
	if err != nil {
		return err
	}
	courses, err := db.Load(ctx)
	if err != nil {
		return err
	}
	byID := map[string]catalog.Course{}
	for _, c := range courses {
		byID[c.ID] = c
	}
	// Validate the entire selection before making any remote requests.
	for _, id := range ids {
		c, exists := byID[id]
		if _, supported := aiCandidateURL(c.Source); !exists || !supported || c.Status != "published" || c.Demo {
			return ErrAI
		}
	}
	if client == nil {
		client = NewClient()
		defer client.CloseIdleConnections()
	}
	results := []aiSourceCheckResult{}
	failed := false
	for _, id := range ids {
		bounded, cancel := context.WithTimeout(ctx, 90*time.Second)
		_, docs, readErr := aiFetchDocuments(bounded, db, client, byID[id])
		cancel()
		r := aiSourceCheckResult{CourseID: id, Code: "documents_ready"}
		if readErr != nil {
			failed = true
			r.Code = rejectionCode(readErr, "source_unavailable_or_contract_rejected")
		} else {
			for _, d := range docs {
				r.TextBytes += len(d.Text)
				r.Partial = r.Partial || d.Partial
				if d.Scope == "course" {
					r.CourseDocuments++
				} else {
					r.OfferDocuments++
				}
			}
			if r.Partial && r.OfferDocuments == 0 {
				r.Code = "partial_course_only_tariff_unverified"
			}
		}
		results = append(results, r)
	}
	if err := json.NewEncoder(w).Encode(map[string]any{"source_check": true, "ok": !failed, "model_requests": 0, "writes": 0, "results": results}); err != nil {
		return err
	}
	if failed {
		return ErrAI
	}
	return nil
}
