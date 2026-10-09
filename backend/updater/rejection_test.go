package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"
)

func TestStepikRejectionReasonsPreserveVerification(t *testing.T) {
	raw, err := os.ReadFile("testdata/stepik-go.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, code string
		change     func(map[string]any)
	}{
		{"wrong identity", "identity_mismatch", func(c map[string]any) { c["id"] = 99 }},
		{"foreign content", "unsupported_content_language", func(c map[string]any) { c["language"] = "en" }},
		{"preview", "insufficient_curriculum", func(c map[string]any) { c["lessons_count"] = 2 }},
		{"private", "private_or_censored_course", func(c map[string]any) { c["is_public"] = false }},
		{"inactive", "inactive_course", func(c map[string]any) { c["is_active"] = false }},
		{"missing flag", "missing_visibility_or_price_flags", func(c map[string]any) { delete(c, "is_paid") }},
		{"contradictory price", "invalid_price_evidence", func(c map[string]any) { c["price"] = 100 }},
		{"irrelevant subject", "unsupported_or_ambiguous_language", func(c map[string]any) { c["title"] = "Math"; c["summary"] = "Algebra" }},
		{"ambiguous subject", "unsupported_or_ambiguous_language", func(c map[string]any) { c["title"] = "Python and Java" }},
		{"missing text", "invalid_course_text", func(c map[string]any) { c["summary"] = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var payload map[string]any
			if json.Unmarshal(raw, &payload) != nil {
				t.Fatal("fixture")
			}
			tc.change(payload["courses"].([]any)[0].(map[string]any))
			body, _ := json.Marshal(payload)
			record, _, err := collectCandidate(body, Candidate{Adapter: "stepik", ExternalID: "54403", URL: "https://stepik.org/course/54403/promo"}, time.Now())
			if !errors.Is(err, ErrSource) || rejectionCode(err, "fallback") != tc.code || record.ID != "" {
				t.Fatalf("record=%s error=%v code=%s", record.ID, err, rejectionCode(err, "fallback"))
			}
		})
	}
	if rejectionCode(errors.New("secret response"), "source_unavailable") != "source_unavailable" {
		t.Fatal("raw error leaked")
	}
}

func TestMissingStructuredCourseHasExplicitReason(t *testing.T) {
	_, _, err := collectCandidate([]byte(`<html>FAQ only</html>`), Candidate{Adapter: "hexlet", ExternalID: "python", URL: "https://ru.hexlet.io/programs/python"}, time.Now())
	if rejectionCode(err, "fallback") != "missing_course_schema" {
		t.Fatal(err)
	}
}

func TestRejectionCodePersistsWithoutPublication(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	config.Discovery = []Feed{{ID: "stepik", Adapter: "stepik", URL: "https://stepik.org/sitemap.xml"}}
	_, err := db.Pool.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state) VALUES('stepik','54403','https://stepik.org/course/54403/promo','stepik','pending')`)
	if err != nil {
		t.Fatal(err)
	}
	service := Service{DB: db, Config: config}
	stats, err := service.processBatch(ctx, clientResponse(200, `{"courses":[{"id":54403,"language":"en"}]}`), func(context.Context, Candidate, catalog.Course, Observation, string) (string, int, error) {
		t.Fatal("rejected course published")
		return "", 0, nil
	})
	if err != nil || stats.Failed != 1 || stats.Published != 0 {
		t.Fatal(stats, err)
	}
	var state, code string
	var failures int
	var backedOff bool
	err = db.Pool.QueryRow(ctx, `SELECT state,code,failures,next_attempt_at>now()+interval '23 hours' FROM catalog_candidates`).Scan(&state, &code, &failures, &backedOff)
	if err != nil || state != "rejected" || code != "unsupported_content_language" || failures != 1 || !backedOff {
		t.Fatal(state, code, failures, backedOff, err)
	}
}
