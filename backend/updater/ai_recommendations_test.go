package updater

import (
	"context"
	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func recommendationExtraction() (Candidate, []AIDocument, AIExtraction, AIVerification) {
	c, docs, _, _ := aiFixture()
	text := "Учимся создавать backend-сервисы на практике, разрабатываем собственный проект и осваиваем язык с базовых конструкций."
	for i := range docs {
		docs[i].Text = text
		docs[i].Digest = fingerprint([]byte(text))
	}
	x := AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{{Field: "recommended_audience", DocumentID: "course", Quote: text, Values: []string{"none", "switch"}}, {Field: "recommended_goals", DocumentID: "course", Quote: text, Values: []string{"try", "switch"}}, {Field: "recommended_directions", DocumentID: "course", Quote: text, Values: []string{"basics", "backend"}}}}
	return c, docs, x, AIVerification{Complete: true, Verified: []int{0, 1, 2}}
}
func TestAIRecommendationsRequireReviewWithoutBecomingFacts(t *testing.T) {
	c, docs, x, v := recommendationExtraction()
	now := time.Now()
	facts, err := aiCompile(c, docs, x, v, now)
	if err != nil || facts.Profile != nil || facts.Recommendations == nil || len(facts.Recommendations.Directions.Values) != 2 {
		t.Fatal(facts, err)
	}
	facts, err = aiCompile(c, docs, x, AIVerification{Complete: true}, now)
	if err != nil || facts.Recommendations != nil {
		t.Fatal("recommendation bypassed review", facts, err)
	}
	for _, mutate := range []func(*AIClaim){func(c *AIClaim) { c.Field = "recommended_price" }, func(c *AIClaim) { c.Values = []string{"mentor"} }, func(c *AIClaim) { c.OfferID = "stepik-42-course"; c.DocumentID = "offer" }, func(c *AIClaim) { c.Available = new(bool) }, func(c *AIClaim) { c.Value = "none" }} {
		bad := x
		bad.Claims = append([]AIClaim(nil), x.Claims...)
		mutate(&bad.Claims[0])
		if _, err = aiCompile(c, docs, bad, v, now); err == nil {
			t.Fatal("unsafe interpretation accepted")
		}
	}
}
func TestAIRecommendationPublicationIdentityManualImportAndCacheRevision(t *testing.T) {
	record, engine, _ := aiImportedCourse(t)
	db := engine.DB
	ctx := context.Background()
	cached := store.NewCached(db)
	_, err := cached.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c, docs, x, v := recommendationExtraction()
	now := time.Now().UTC().Truncate(time.Microsecond)
	e := AIEvent{Version: 1, CourseID: record.ID, IdentityDigest: aiIdentityDigest(record), Candidate: c, ObservedAt: now, Model: "deepseek:test", Policy: aiEffectivePolicy(), Documents: docs, Extraction: x, Verification: v}
	s := Service{DB: db}
	if err = s.enqueueAI(ctx, e); err != nil {
		t.Fatal(err)
	}
	if n, err := s.publishAI(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	loaded, err := cached.Load(ctx)
	if err != nil || loaded[0].Recommendations == nil || loaded[0].Profile != nil || aiProtectedDigest(record) != aiProtectedDigest(loaded[0]) {
		t.Fatal(loaded, err)
	}
	coverage, err := aiCoverage(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range coverage {
		if v.Provider == "stepik" && (v.UnclassifiedProfile != 0 || v.RecommendedProfile["experience"] != 1 || v.RecommendedProfile["goal"] != 1 || v.RecommendedProfile["direction"] != 1) {
			t.Fatal(v)
		}
	}
	if loaded[0].RecommendationIdentity != catalog.CourseIdentityDigest(loaded[0]) {
		t.Fatal("identity binding missing")
	}
	// A price refresh does not change learner suitability.
	_, err = db.UpdateVerified(ctx, nil, "native-test", func(_ context.Context, _ pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
		c := old[0]
		c.Offers = append([]catalog.Offer(nil), c.Offers...)
		p := *c.Offers[0].Price + 1
		c.Offers[0].Price = &p
		return []catalog.Course{c}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = db.Load(ctx)
	if err != nil || loaded[0].Recommendations == nil {
		t.Fatal("repricing erased estimate", err)
	}
	// A pending interpretation cannot cross a changed course identity.
	e.ObservedAt = now.Add(time.Second)
	if err = s.enqueueAI(ctx, e); err != nil {
		t.Fatal(err)
	}
	_, err = db.UpdateVerified(ctx, nil, "native-test", func(_ context.Context, _ pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
		c := old[0]
		c.Title += " changed"
		return []catalog.Course{c}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.publishAI(ctx); err != nil || n != 0 {
		t.Fatal(n, err)
	}
	loaded, err = db.Load(ctx)
	if err != nil || loaded[0].Recommendations != nil {
		t.Fatal("stale identity survived", err)
	}
	// Operator imports cannot fabricate a reviewed AI interpretation.
	loaded[0].Recommendations = &catalog.CourseRecommendations{Audience: &catalog.RecommendationTags{Values: []string{"none"}, Quote: x.Claims[0].Quote, Evidence: catalog.LearningEvidence{Source: c.URL, Locator: "test", Digest: fingerprint([]byte("test")), CheckedAt: now}}}
	loaded[0].RecommendationIdentity = catalog.CourseIdentityDigest(loaded[0])
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"stepik.org"}, Courses: loaded}, "operator", false); err != nil {
		t.Fatal(err)
	}
	loaded, err = db.Load(ctx)
	if err != nil || loaded[0].Recommendations != nil {
		t.Fatal("operator input fabricated estimates", err)
	}
}

type recommendationModel struct{ calls int }

func (m *recommendationModel) Extract(_ context.Context, c Candidate, docs []AIDocument) (AIExtraction, AIUsage, error) {
	m.calls++
	_, _, x, _ := recommendationExtraction()
	x.CourseURL = c.URL
	for _, d := range docs {
		if d.Scope == "course" {
			for i := range x.Claims {
				x.Claims[i].DocumentID = d.ID
				x.Claims[i].Quote = aiNormalize(d.Text)
			}
		}
	}
	return x, AIUsage{Prompt: 10, Completion: 10, Total: 20}, nil
}
func (m *recommendationModel) Verify(_ context.Context, _ Candidate, _ []AIDocument, _ AIExtraction) (AIVerification, AIUsage, error) {
	m.calls++
	return AIVerification{Complete: true, Verified: []int{0, 1, 2}}, AIUsage{Prompt: 5, Completion: 5, Total: 10}, nil
}
func TestAIRecommendationPreviewSharesBudgetWithoutPublishing(t *testing.T) {
	record, engine, _ := aiImportedCourse(t)
	ctx := context.Background()
	model := &recommendationModel{}
	service := AIService{DB: engine.DB, Model: model, Client: aiSourceClient(200), Config: AIConfig{Model: "fake", VerifyModel: "review", BatchSize: 1, DailyLimit: 2}}
	for n := 0; n < 2; n++ {
		r, err := service.PreviewCourse(ctx, record.ID)
		if err != nil || r.Recommendations == nil || r.CatalogWrites != 0 || r.Cached != (n == 1) {
			t.Fatal(r, err)
		}
	}
	if model.calls != 2 {
		t.Fatal("preview bypassed shared cache", model.calls)
	}
	var imports, queued, attempted, requests int
	if err := engine.DB.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM imports),(SELECT count(*) FROM catalog_ai_observations),(SELECT count(*) FROM catalog_ai_sources),(SELECT requests FROM catalog_ai_daily_budget)`).Scan(&imports, &queued, &attempted, &requests); err != nil {
		t.Fatal(err)
	}
	if imports != 1 || queued != 0 || attempted != 0 || requests != 2 {
		t.Fatal(imports, queued, attempted, requests)
	}
	loaded, err := engine.DB.Load(ctx)
	if err != nil || loaded[0].Recommendations != nil {
		t.Fatal("preview changed public data", err)
	}
}
