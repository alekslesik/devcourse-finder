package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAITargetedVerificationOnlyPublishesOwnObservationAndPreservesBudget(t *testing.T) {
	record, engine, _ := aiImportedCourse(t)
	ctx := context.Background()
	// An older queued event for this same course must not be drained by verify.
	c, docs, x, v := aiFixture()
	publisher := Service{DB: engine.DB}
	old := AIEvent{Version: 1, CourseID: record.ID, IdentityDigest: aiIdentityDigest(record), Candidate: c, ObservedAt: time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond), Model: "test", Policy: aiEffectivePolicy(), Documents: docs, Extraction: x, Verification: v}
	if err := publisher.enqueueAI(ctx, old); err != nil {
		t.Fatal(err)
	}
	model := &adaptiveAIModel{}
	s := AIService{DB: engine.DB, Model: model, Client: aiSourceClient(200), Config: AIConfig{Model: "test", VerifyModel: "test", BatchSize: 20, DailyLimit: 2}}
	report, err := s.VerifyCourse(ctx, record.ID)
	if err != nil || report.Run.Attempted != 1 || report.Run.Queued != 1 || !report.FinancialUnchanged || report.Code != "ai_verified" || len(report.VerifiedClaims) != 2 {
		t.Fatal(report, err)
	}
	var pending int
	if err = engine.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM catalog_ai_observations WHERE processed_at IS NULL`).Scan(&pending); err != nil || pending != 1 {
		t.Fatal("unrelated pending event drained", pending, err)
	}
	// Explicit target rechecks ignore the due time but retain the unchanged-content
	// cache and the same exhausted shared cap; a repeat must make zero model calls.
	calls := model.calls
	again, err := s.VerifyCourse(ctx, record.ID)
	if err != nil || again.Run.Cached != 1 || model.calls != calls || !again.FinancialUnchanged {
		t.Fatal(again, err)
	}
	var requests int
	if err = engine.DB.Pool.QueryRow(ctx, `SELECT requests FROM catalog_ai_daily_budget`).Scan(&requests); err != nil || requests != 2 {
		t.Fatal(requests, err)
	}
	if _, err = s.VerifyCourse(ctx, "missing-course"); err == nil || model.calls != calls {
		t.Fatal("missing target accepted")
	}
}
func TestAITargetedHexletVerificationDoesNotProbeStepikOrInferOperatorOffers(t *testing.T) {
	db := postgresFixture(t)
	_, c, r := hexletPublicFixture(t, "java")
	ctx := context.Background()
	raw, err := os.ReadFile("testdata/hexlet-public-java-tariffs.html")
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic profile quote controls model plumbing, not real semantic accuracy.
	raw = []byte(strings.Replace(string(raw), `<meta name="description" content="`, `<meta name="description" content="Программа предназначена для начинающих без опыта программирования. `, 1))
	extra := r.Offers[0]
	extra.ID += "-operator"
	r.Offers = append(r.Offers, extra)
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"ru.hexlet.io"}, Courses: []catalog.Course{r}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	model := &publicNetologyModel{}
	client := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != c.URL {
			t.Fatal("off-target network call", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	s := AIService{DB: db, Model: model, Client: client, Config: AIConfig{Model: "test", VerifyModel: "test", BatchSize: 20, DailyLimit: 2}}
	report, err := s.VerifyCourse(ctx, r.ID)
	if err != nil || len(report.Documents) != 4 || !report.FinancialUnchanged {
		t.Fatal(report, err)
	}
	for _, d := range report.Documents {
		if strings.Contains(d["offer_id"].(string), "-operator") {
			t.Fatal("operator offer inferred")
		}
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), "api_key") {
		t.Fatal("secret material in report")
	}
}
func TestAIProtectedDigestDetectsFinancialIdentityAndFreshnessChanges(t *testing.T) {
	_, _, r, _ := aiNetologyFixture(t)
	before := aiProtectedDigest(r)
	for _, change := range []func(*catalog.Course){func(c *catalog.Course) { c.CheckedAt = c.CheckedAt.Add(time.Second) }, func(c *catalog.Course) { c.Offers[0].PriceCheckedAt = c.Offers[0].PriceCheckedAt.Add(time.Second) }, func(c *catalog.Course) { c.Offers[0].Enrollment = "closed" }, func(c *catalog.Course) { c.Offers[0].URL += "/other" }, func(c *catalog.Course) { p := *c.Offers[0].Price + 1; c.Offers[0].Price = &p }} {
		copy := r
		copy.Offers = append([]catalog.Offer(nil), r.Offers...)
		change(&copy)
		if aiProtectedDigest(copy) == before {
			t.Fatal("protected change ignored")
		}
	}
}
