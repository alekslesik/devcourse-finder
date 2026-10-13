package updater

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"devcourse-finder/catalog"
)

type adaptiveAIModel struct{ fakeAIModel }

func (m *adaptiveAIModel) Extract(ctx context.Context, c Candidate, docs []AIDocument) (AIExtraction, AIUsage, error) {
	x, u, err := m.fakeAIModel.Extract(ctx, c, docs)
	x.CourseURL = c.URL
	for i := range x.Claims {
		for _, d := range docs {
			if x.Claims[i].Field == "audience" && d.Scope == "course" {
				x.Claims[i].DocumentID = d.ID
			}
			if x.Claims[i].Field == "weekly_workload" && d.Scope == "offer" {
				x.Claims[i].DocumentID = d.ID
				x.Claims[i].OfferID = d.OfferID
			}
		}
	}
	return x, u, err
}
func aiSourceClient(status int) *http.Client {
	remote := validStepik()
	remote["id"] = 42
	remote["title"] = "Python курс"
	remote["language"] = "ru"
	_, docs, _, _ := aiFixture()
	remote["description"] = docs[0].Text
	body := string(stepikJSON(remote))
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
}
func TestAIWorkerPipelineCacheStatusAndFailedReadDates(t *testing.T) {
	record, engine, _ := aiImportedCourse(t)
	ctx := context.Background()
	model := &adaptiveAIModel{}
	service := AIService{DB: engine.DB, Config: AIConfig{Model: "gpt-4.1-mini", BatchSize: 20, DailyLimit: 4}, Model: model, Client: aiSourceClient(200)}
	first, err := service.Run(ctx)
	if err != nil || first.Queued != 1 || first.Failed != 0 || first.Tokens != 150 {
		t.Fatal(first, err)
	}
	publisher := Service{DB: engine.DB, Config: productionConfig(t), Worker: "publisher"}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ := engine.DB.Load(ctx)
	if loaded[0].Profile == nil {
		t.Fatal(loaded)
	}
	// Force a manual second eligible window; identical content spends no API calls.
	engine.DB.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now() WHERE course_id=$1", record.ID)
	second, err := service.Run(ctx)
	if err != nil || second.Cached != 1 || model.calls != 2 {
		t.Fatal(second, err, model.calls)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = engine.DB.Load(ctx)
	at := loaded[0].Profile.Audience.Evidence.CheckedAt
	engine.DB.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now() WHERE course_id=$1", record.ID)
	service.Client = aiSourceClient(404)
	failed, err := service.Run(ctx)
	if err != nil || failed.Failed != 1 || failed.Queued != 0 {
		t.Fatal(failed, err)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = engine.DB.Load(ctx)
	if !loaded[0].Profile.Audience.Evidence.CheckedAt.Equal(at) {
		t.Fatal("failed source advanced facts")
	}
	var status bytes.Buffer
	if err = AIStatus(ctx, engine.DB, &status); err != nil || !strings.Contains(status.String(), `"requests_reserved_today":2`) || !strings.Contains(status.String(), "source_unavailable") {
		t.Fatal(status.String(), err)
	}
	if err = heartbeatWorker(ctx, engine.DB, "ai"); err != nil {
		t.Fatal(err)
	}
	if err = AIHealth(ctx, engine.DB); err != nil {
		t.Fatal(err)
	}
}
func TestAIWorkerExcludesDemoAndOtherProviders(t *testing.T) {
	record, engine, _ := aiImportedCourse(t)
	ctx := context.Background()
	record.Demo = true
	if err := engine.DB.Import(ctx, catalog.Dataset{Domains: []string{"stepik.org"}, Courses: []catalog.Course{record}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	service := AIService{DB: engine.DB, Config: AIConfig{Model: "gpt-4.1-mini", BatchSize: 20, DailyLimit: 4}, Model: &adaptiveAIModel{}, Client: aiSourceClient(200)}
	result, err := service.Run(ctx)
	if err != nil || result.Attempted != 0 {
		t.Fatal(result, err)
	}
}
