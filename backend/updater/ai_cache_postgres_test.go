package updater

import (
	"context"
	"devcourse-finder/catalog"
	"fmt"
	"testing"
	"time"
)

type fakeAIModel struct {
	calls   int
	failure bool
}

func (m *fakeAIModel) Extract(ctx context.Context, c Candidate, d []AIDocument) (AIExtraction, AIUsage, error) {
	m.calls++
	_, _, x, _ := aiFixture()
	if m.failure {
		return x, AIUsage{}, ErrAI
	}
	return x, AIUsage{Prompt: 100, Total: 100}, nil
}
func (m *fakeAIModel) Verify(ctx context.Context, c Candidate, d []AIDocument, x AIExtraction) (AIVerification, AIUsage, error) {
	m.calls++
	return AIVerification{Complete: true, Verified: []int{0, 1}}, AIUsage{Prompt: 50, Total: 50}, nil
}
func aiImportedCourse(t *testing.T) (catalog.Course, *AIEngine, []AIDocument) {
	t.Helper()
	db := postgresFixture(t)
	c, docs, _, _ := aiFixture()
	now := time.Now().UTC().Truncate(time.Microsecond)
	price := int64(100000)
	record, err := normalizedRecord(c, "Python курс", "Изучение Python для начинающих", "Stepik", Observation{Price: &price, Enrollment: "continuous", Schedule: "flexible"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Import(context.Background(), catalog.Dataset{Domains: []string{"stepik.org"}, Courses: []catalog.Course{record}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	return record, &AIEngine{DB: db, Model: &fakeAIModel{}, ModelName: "gpt-4.1-mini", DailyLimit: 2}, docs
}
func TestAICacheUnchangedContentAndPersistentBudget(t *testing.T) {
	record, engine, docs := aiImportedCourse(t)
	c, _, _, _ := aiFixture()
	ctx := context.Background()
	first, err := engine.Resolve(ctx, record.ID, c, docs, time.Now())
	if err != nil || first.Cached {
		t.Fatal(first, err)
	}
	docs[0].Digest = fingerprint([]byte("changed transport metadata, identical extracted text"))
	second, err := engine.Resolve(ctx, record.ID, c, docs, time.Now())
	if err != nil || !second.Cached || engine.Model.(*fakeAIModel).calls != 2 {
		t.Fatal(second, err)
	}
	docs[0].Text += " Новая информация о программе."
	if _, err = engine.Resolve(ctx, record.ID, c, docs, time.Now()); err != ErrAIBudget {
		t.Fatal("budget not durable", err)
	}
	var requests, tokens int
	if err = engine.DB.Pool.QueryRow(ctx, "SELECT requests,total_tokens FROM catalog_ai_daily_budget").Scan(&requests, &tokens); err != nil || requests != 2 || tokens != 150 {
		t.Fatal(requests, tokens, err)
	}
}
func TestAIFailedRequestsConsumeBudgetAndDoNotCache(t *testing.T) {
	record, engine, docs := aiImportedCourse(t)
	engine.Model = &fakeAIModel{failure: true}
	engine.DailyLimit = 2
	c, _, _, _ := aiFixture()
	if _, err := engine.Resolve(context.Background(), record.ID, c, docs, time.Now()); err != ErrAI {
		t.Fatal(err)
	}
	if _, err := engine.Resolve(context.Background(), record.ID, c, docs, time.Now()); err != ErrAIBudget {
		t.Fatal(err)
	}
	var count int
	engine.DB.Pool.QueryRow(context.Background(), "SELECT count(*) FROM catalog_ai_cache").Scan(&count)
	if count != 0 {
		t.Fatal("failure cached")
	}
}

func TestAICompletePairRequiredBeforePaidExtraction(t *testing.T) {
	for _, limit := range []int{1, 3} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			record, engine, docs := aiImportedCourse(t)
			engine.DailyLimit = limit
			c, _, _, _ := aiFixture()
			if limit == 3 {
				if _, err := engine.Resolve(context.Background(), record.ID, c, docs, time.Now()); err != nil {
					t.Fatal(err)
				}
				docs[0].Text += " Новый раздел программы."
			}
			before := engine.Model.(*fakeAIModel).calls
			if _, err := engine.Resolve(context.Background(), record.ID, c, docs, time.Now()); err != ErrAIBudget {
				t.Fatal(err)
			}
			if engine.Model.(*fakeAIModel).calls != before {
				t.Fatal("paid extraction started without a complete request pair")
			}
			var requests int
			engine.DB.Pool.QueryRow(context.Background(), "SELECT coalesce(sum(requests),0) FROM catalog_ai_daily_budget").Scan(&requests)
			if requests != before {
				t.Fatalf("reservation changed: got %d want %d", requests, before)
			}
		})
	}
}

// The invalid first claim must neither poison the course nor shift the verifier's
// indices; only independently approved, surviving claims may reach publication.
type mixedAIModel struct{ calls int }

func (m *mixedAIModel) Extract(ctx context.Context, c Candidate, docs []AIDocument) (AIExtraction, AIUsage, error) {
	m.calls++
	_, _, x, _ := aiFixture()
	bad := x.Claims[1]
	bad.Field = "duration"
	bad.Unit = "month"
	bad.Max = valueFloat(99)
	badDocument := AIClaim{Field: "prerequisites", DocumentID: "nonexistent", QuoteID: "s0", Value: "none", Scope: "programming"}
	badQuote := AIClaim{Field: "excluded_audience", DocumentID: "course", QuoteID: "nonexistent", Values: []string{"switch"}}
	x.Claims[0].QuoteID = "s0"
	x.Claims[0].Quote = "" // Must be copied locally from the source before review.
	x.Claims = append([]AIClaim{bad, badDocument, badQuote}, x.Claims...)
	return x, AIUsage{Prompt: 100, Total: 100}, nil
}
func (m *mixedAIModel) Verify(ctx context.Context, c Candidate, docs []AIDocument, x AIExtraction) (AIVerification, AIUsage, error) {
	m.calls++
	if len(x.Claims) != 2 || x.DiscardedClaims != 3 || x.Claims[0].Field != "audience" || *x.Claims[1].Max != 12 {
		return AIVerification{}, AIUsage{}, ErrAI
	}
	return AIVerification{Complete: true, Verified: []int{0}}, AIUsage{Prompt: 50, Total: 50}, nil
}
func TestAIPartialAnalysisCachesOnlyIndependentlyVerifiedFacts(t *testing.T) {
	record, engine, docs := aiImportedCourse(t)
	c, _, _, _ := aiFixture()
	model := &mixedAIModel{}
	engine.Model = model
	first, err := engine.Resolve(context.Background(), record.ID, c, docs, time.Now())
	if err != nil || first.Cached || first.Extraction.DiscardedClaims != 3 {
		t.Fatal(first, err)
	}
	facts, err := aiCompile(c, docs, first.Extraction, first.Verification, time.Now())
	if err != nil || facts.Profile == nil || facts.Learning[docs[1].OfferID] != nil {
		t.Fatal(facts, err)
	}
	second, err := engine.Resolve(context.Background(), record.ID, c, docs, time.Now())
	if err != nil || !second.Cached || second.Extraction.DiscardedClaims != 3 || model.calls != 2 {
		t.Fatal("partial flag or cache lost", second, err, model.calls)
	}
}
