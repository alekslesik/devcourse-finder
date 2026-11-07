package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRecommendationOnlyProviderContractRejectsLiteralFacts(t *testing.T) {
	c, docs, x, _ := recommendationExtraction()
	calls := 0
	model := &DeepSeekModel{Key: "test", Model: "test", recommendationsOnly: true}
	model.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(body.Messages[0].Content, `"field":"audience"`) {
			t.Fatal("literal example leaked into recommendations-only contract")
		}
		var input map[string]any
		if err := json.Unmarshal([]byte(body.Messages[1].Content), &input); err != nil || input["recommendations_only"] != true {
			t.Fatal(input, err)
		}
		result := x
		if calls == 2 {
			result.Claims = append(result.Claims, AIClaim{Field: "prerequisites", DocumentID: "course", QuoteID: "s0", Value: "none", Scope: "programming"})
		}
		raw, _ := json.Marshal(result)
		envelope, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(raw)}}}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(envelope)))}, nil
	})}
	if _, _, err := model.Extract(context.Background(), c, docs); err != nil {
		t.Fatal(err)
	}
	if _, _, err := model.Extract(context.Background(), c, docs); err == nil {
		t.Fatal("literal fact escaped recommendations-only provider")
	}
}
func TestRecommendationOnlyPublicationPreservesAllFacts(t *testing.T) {
	record, engine, _ := aiImportedCourse(t)
	ctx := context.Background()
	c, docs, x, v := recommendationExtraction()
	docs = AIService{Config: AIConfig{RecommendationsOnly: true}}.aiDocuments(docs)
	_, factDocs, factX, factV := aiFixture()
	facts, err := aiCompile(c, factDocs, factX, factV, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.DB.UpdateVerified(ctx, nil, "native-fixture", func(_ context.Context, _ pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
		r := old[0]
		r.Profile = facts.Profile
		r.ProfileOwnership = profileAudience | (profileAudience << aiOwnershipShift)
		r.Audience = []string{"none"}
		r.Offers = append([]catalog.Offer(nil), r.Offers...)
		r.Offers[0].LearningConditions = facts.Learning["stepik-42-course"]
		r.Offers[0].LearningOwnership = learningWorkload | (learningWorkload << aiOwnershipShift)
		return []catalog.Course{r}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := engine.DB.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	event := AIEvent{RecommendationsOnly: true, Version: 1, CourseID: record.ID, IdentityDigest: aiIdentityDigest(before[0]), Candidate: c, ObservedAt: time.Now().UTC().Truncate(time.Microsecond), Model: "deepseek:test:recommendations", Policy: aiEffectivePolicy(), Documents: docs, Extraction: x, Verification: v}
	publisher := Service{DB: engine.DB}
	if err = publisher.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if n, err := publisher.publishAI(ctx); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	after, err := engine.DB.Load(ctx)
	if err != nil || after[0].Recommendations == nil || aiRecommendationProtectedDigest(before[0]) != aiRecommendationProtectedDigest(after[0]) {
		t.Fatal("protected facts changed", err)
	}
	badScope := event
	_, badScope.Documents, _, _ = recommendationExtraction()
	if badScope.valid() {
		t.Fatal("offer document escaped recommendation event boundary")
	}
	// An empty later analysis is not proof that a prior supported label disappeared.
	event.ObservedAt = event.ObservedAt.Add(time.Second)
	event.Extraction.Claims = nil
	event.Verification.Verified = nil
	if err = publisher.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err = publisher.publishAI(ctx); err != nil {
		t.Fatal(err)
	}
	retained, err := engine.DB.Load(ctx)
	if err != nil || retained[0].Recommendations == nil {
		t.Fatal("abstention erased prior advice", err)
	}
	event.Extraction.Claims = []AIClaim{{Field: "audience", DocumentID: "course", Quote: x.Claims[0].Quote, Values: []string{"none"}}}
	event.Verification.Verified = []int{0}
	if event.valid() {
		t.Fatal("literal fact escaped event boundary")
	}
}
func TestRecommendationOnlyPreviewCacheAndCarryForwardExcludeFacts(t *testing.T) {
	record, engine, _ := aiImportedCourse(t)
	ctx := context.Background()
	c, docs, x, _ := recommendationExtraction()
	facts := x
	facts.Claims = []AIClaim{{Field: "audience", DocumentID: "course", Quote: docs[0].Text, Values: []string{"none"}}}
	rawX, _ := json.Marshal(facts)
	rawV, _ := json.Marshal(AIVerification{Complete: true, Verified: []int{0}})
	_, err := engine.DB.Pool.Exec(ctx, "INSERT INTO catalog_ai_cache(course_id,bundle_digest,model,policy,extraction,verification) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb)", record.ID, aiBundleDigest(docs), "older-mixed", aiEffectivePolicy(), rawX, rawV)
	if err != nil {
		t.Fatal(err)
	}
	engine.RecommendationsOnly = true
	carried, err := engine.carryForward(ctx, record.ID, c, docs, AIExtraction{CourseURL: c.URL, Complete: true}, time.Now())
	if err != nil || len(carried.Claims) != 0 {
		t.Fatal("literal claim carried into recommendation lane", carried, err)
	}
	engine.ModelName = "mixed-cache-poison"
	_, err = engine.DB.Pool.Exec(ctx, "UPDATE catalog_ai_cache SET model=$1", engine.ModelName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = engine.Resolve(ctx, record.ID, c, docs, time.Now()); err == nil {
		t.Fatal("literal cache entry accepted in recommendation lane")
	}
}
func TestRecommendationModeConfigurationBounds(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test")
	t.Setenv("CATALOG_AI_RECOMMENDATIONS_ONLY", "true")
	t.Setenv("CATALOG_AI_BATCH_SIZE", "100")
	t.Setenv("CATALOG_AI_DAILY_REQUEST_LIMIT", "200")
	c, err := LoadAIConfig()
	if err != nil || !c.RecommendationsOnly || c.BatchSize != 100 || c.DailyLimit != 200 {
		t.Fatal(c.BatchSize, err)
	}
	for key, value := range map[string]string{"CATALOG_AI_RECOMMENDATIONS_ONLY": "invalid", "CATALOG_AI_BATCH_SIZE": "101", "CATALOG_AI_DAILY_REQUEST_LIMIT": "201"} {
		t.Run(key, func(t *testing.T) {
			t.Setenv(key, value)
			if _, err := LoadAIConfig(); err == nil {
				t.Fatal("invalid configuration accepted")
			}
		})
	}
	service := AIService{Config: AIConfig{RecommendationsOnly: true}}
	if service.aiModeSuffix() == "" {
		t.Fatal("cache lane missing")
	}
	_, docs, _, _ := aiFixture()
	filtered := service.aiDocuments(docs)
	if len(filtered) != 1 || filtered[0].Scope != "course" {
		t.Fatal("offer evidence sent to recommendation task")
	}
}
