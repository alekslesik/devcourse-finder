package updater

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

type netologyAIModel struct{ calls int }

func (m *netologyAIModel) Extract(ctx context.Context, c Candidate, docs []AIDocument) (AIExtraction, AIUsage, error) {
	m.calls++
	x := AIExtraction{CourseURL: c.URL, Complete: true}
	for _, d := range docs {
		if d.Scope == "course" {
			x.Claims = append(x.Claims, AIClaim{Field: "audience", DocumentID: d.ID, Quote: "Программа предназначена для начинающих без опыта программирования.", Values: []string{"none"}})
		}
		if d.Scope == "offer" {
			x.Claims = append(x.Claims, AIClaim{Field: "duration", DocumentID: d.ID, OfferID: d.OfferID, Quote: "9 месяцев и 278 часов практики", Min: valueFloat(9), Max: valueFloat(9), Unit: "month"})
		}
	}
	return x, AIUsage{Prompt: 100, Total: 100}, nil
}
func (m *netologyAIModel) Verify(ctx context.Context, c Candidate, docs []AIDocument, x AIExtraction) (AIVerification, AIUsage, error) {
	m.calls++
	if len(x.Claims) != 2 {
		return AIVerification{}, AIUsage{}, ErrAI
	}
	return AIVerification{Complete: true, Verified: []int{0, 1}}, AIUsage{Prompt: 50, Total: 50}, nil
}

func TestAINetologyPipelineCacheIdentityFailuresAndManualTariffs(t *testing.T) {
	db := postgresFixture(t)
	raw, c, _, _ := aiNetologyFixture(t)
	raw = netologyCurrent(t, raw, time.Now().UTC())
	record, o, err := collectNetology(raw, c, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	record.Profile = nil
	record.Audience = []string{"unknown"}
	record.Goals = []string{"unknown"}
	record.Direction = "unknown"
	record.Offers[0].LearningConditions = nil
	record.Offers[0].DurationMonths = nil
	record.Offers[0].Weeks = nil
	record.Offers[0].Hours = nil
	extra := record.Offers[0]
	extra.ID += "-manual-plus"
	extra.Name = "Manual Plus"
	record.Offers = append(record.Offers, extra)
	ctx := context.Background()
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"netology.ru"}, Courses: []catalog.Course{record}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	initial, loadErr := db.Load(ctx)
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	record = initial[0]
	_, err = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id,netology_family_id,netology_program_id) VALUES('netology',$1,$2,$3,$4,$5,$6)`, c.ExternalID, c.URL, record.ID, record.Offers[0].ID, o.NetologyFamilyID, o.NetologyProgramID)
	if err != nil {
		t.Fatal(err)
	}
	model := &netologyAIModel{}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != candidateEndpoint(c) {
			t.Fatal("unexpected fetch", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	service := AIService{DB: db, Config: AIConfig{Model: "test-extract", VerifyModel: "test-verify", BatchSize: 1, DailyLimit: 2}, Model: model, Client: client}
	first, err := service.Run(ctx)
	if err != nil || first.Queued != 1 || first.Tokens != 150 || first.Failed != 0 {
		t.Fatal(first, err)
	}
	publisher := Service{DB: db, Config: productionConfig(t), Worker: "publisher"}
	// A changed mapping between collection and publication must reject the
	// queued event even when the public course fields/URL did not change.
	if _, err = db.Pool.Exec(ctx, "UPDATE catalog_identities SET netology_family_id=netology_family_id+1 WHERE course_id=$1", record.ID); err != nil {
		t.Fatal(err)
	}
	rejected, err := publisher.Publish(ctx)
	if err != nil || rejected.RefreshedCourses != 0 {
		t.Fatal("changed queued identity published", rejected, err)
	}
	rejectedRows, err := db.Load(ctx)
	if err != nil || rejectedRows[0].Profile != nil {
		t.Fatal("protected identity acquired AI facts", err)
	}
	var code string
	if err = db.Pool.QueryRow(ctx, "SELECT code FROM catalog_ai_observations ORDER BY id DESC LIMIT 1").Scan(&code); err != nil || code != "protected_ai_identity" {
		t.Fatal(code, err)
	}
	if _, err = db.Pool.Exec(ctx, "UPDATE catalog_identities SET netology_family_id=$2 WHERE course_id=$1", record.ID, o.NetologyFamilyID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now() WHERE course_id=$1", record.ID); err != nil {
		t.Fatal(err)
	}
	recovered, err := service.Run(ctx)
	if err != nil || recovered.Cached != 1 || model.calls != 2 {
		t.Fatal("recovery spent extra requests", recovered, err)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.Load(ctx)
	if err != nil || len(loaded) != 1 {
		t.Fatal(loaded, err)
	}
	a := loaded[0]
	if a.Profile == nil || a.Profile.Audience == nil || a.ProfileOwnership&(profileAudience<<aiOwnershipShift) == 0 {
		t.Fatal("profile not accepted", a)
	}
	if a.Offers[0].LearningConditions == nil || a.Offers[0].LearningConditions.Duration.Max != 9 || a.Offers[1].LearningConditions != nil {
		t.Fatal("wrong tariff enriched", a.Offers)
	}
	if *a.Offers[0].Price != *record.Offers[0].Price || !a.Offers[0].PriceCheckedAt.Equal(record.Offers[0].PriceCheckedAt) || !a.CheckedAt.Equal(record.CheckedAt) {
		t.Fatal("AI changed financial freshness")
	}
	_, err = db.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now() WHERE course_id=$1", record.ID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Run(ctx)
	if err != nil || second.Cached != 1 || model.calls != 2 {
		t.Fatal("cache spent extra requests", second, err, model.calls)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = db.Load(ctx)
	verifiedAt := loaded[0].Profile.Audience.Evidence.CheckedAt
	raw = netologyMutate(t, raw, func(d map[string]any) { netologyProgramMap(d)["program_family_id"] = float64(o.NetologyFamilyID + 1) })
	db.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now() WHERE course_id=$1", record.ID)
	failed, err := service.Run(ctx)
	if err != nil || failed.Failed != 1 || failed.Tokens != 0 || model.calls != 2 {
		t.Fatal("changed family spent requests", failed, err)
	}
	var status bytes.Buffer
	if err = AIStatus(ctx, db, &status); err != nil || !strings.Contains(status.String(), `"supported_providers":["netology","stepik","yandex"]`) {
		t.Fatal(status.String(), err)
	}
	loaded, _ = db.Load(ctx)
	if !loaded[0].Profile.Audience.Evidence.CheckedAt.Equal(verifiedAt) {
		t.Fatal("failed identity refreshed evidence")
	}
}
