package updater

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"devcourse-finder/catalog"
)

type basicsAIModel struct{ calls int }

func (m *basicsAIModel) Extract(ctx context.Context, c Candidate, docs []AIDocument) (AIExtraction, AIUsage, error) {
	m.calls++
	return AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{{Field: "audience", DocumentID: "course", Quote: "Курс разработан для начинающих без опыта программирования.", Values: []string{"none"}}}}, AIUsage{Prompt: 100, Total: 100}, nil
}
func (m *basicsAIModel) Verify(ctx context.Context, c Candidate, docs []AIDocument, x AIExtraction) (AIVerification, AIUsage, error) {
	m.calls++
	return AIVerification{Complete: true, Verified: []int{0}}, AIUsage{Prompt: 50, Total: 50}, nil
}

func TestAICodeBasicsPipelineCacheBillingPrivacyAndQueuedBinding(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	raw, policy, c, record := aiBasicsFixture(t)
	record.Title = "Курс Python"
	record.Offers[0].Name = "Бесплатная полная программа"
	record.Profile = nil
	record.Audience = []string{"unknown"}
	record.Goals = []string{"unknown"}
	record.Direction = "unknown"
	record.Offers[0].LearningConditions = nil
	extra := record.Offers[0]
	extra.ID += "-manual"
	extra.Name = "Manual extra"
	record.Offers = append(record.Offers, extra)
	if err := db.Import(ctx, catalog.Dataset{Domains: []string{"code-basics.com"}, Courses: []catalog.Course{record}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	initial, err := db.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record = initial[0]
	_, err = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id) VALUES('codebasics',$1,$2,$3,$4)`, c.ExternalID, c.URL, record.ID, record.Offers[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	model := &basicsAIModel{}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		data := raw
		switch r.URL.String() {
		case candidateEndpoint(c):
		case "https://code-basics.com/ru":
			data = policy
		default:
			t.Fatal("unexpected fetch", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(data)))}, nil
	})}
	service := AIService{DB: db, Config: AIConfig{Model: "test-extract", VerifyModel: "test-verify", BatchSize: 1, DailyLimit: 2}, Model: model, Client: client}
	first, err := service.Run(ctx)
	if err != nil || first.Queued != 1 || first.Tokens != 150 {
		t.Fatal(first, err)
	}
	// Reject a source identity remap made after the model response was queued.
	db.Pool.Exec(ctx, "UPDATE catalog_identities SET offer_id=$2 WHERE course_id=$1", record.ID, extra.ID)
	publisher := Service{DB: db, Config: productionConfig(t), Worker: "publisher"}
	result, err := publisher.Publish(ctx)
	if err != nil || result.RefreshedCourses != 0 {
		t.Fatal("remapped source published", result, err)
	}
	db.Pool.Exec(ctx, "UPDATE catalog_identities SET offer_id=$2 WHERE course_id=$1", record.ID, record.Offers[0].ID)
	db.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now() WHERE course_id=$1", record.ID)
	second, err := service.Run(ctx)
	if err != nil || second.Cached != 1 || model.calls != 2 {
		t.Fatal("cached recovery spent requests", second, err)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.Load(ctx)
	if err != nil || len(loaded) != 1 {
		t.Fatal(loaded, err)
	}
	current := loaded[0]
	if current.Profile == nil || current.Profile.Audience == nil || current.Profile.Audience.Values[0] != "none" || current.ProfileOwnership&(profileAudience<<aiOwnershipShift) == 0 {
		t.Fatal("profile not enriched", current)
	}
	if *current.Offers[0].Price != 0 || !current.Offers[0].Free || !current.Offers[0].PriceCheckedAt.Equal(record.Offers[0].PriceCheckedAt) || !current.CheckedAt.Equal(record.CheckedAt) {
		t.Fatal("billing or freshness changed")
	}
	if current.Offers[0].LearningConditions != nil || current.Offers[1].LearningConditions != nil {
		t.Fatal("total hours became duration or manual offer enriched")
	}
	// Changed policy blocks the source before cache use or a new model reservation.
	policy = []byte("Pricing policy no longer confirms the course contract")
	db.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now() WHERE course_id=$1", record.ID)
	failed, err := service.Run(ctx)
	if err != nil || failed.Failed != 1 || failed.Tokens != 0 || model.calls != 2 {
		t.Fatal(failed, err)
	}
}
