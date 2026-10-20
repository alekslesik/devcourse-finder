package updater

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"devcourse-finder/catalog"
)

func TestAITargetedRunExcludesUnselectedAndRetainsCooldownAndSharedBudget(t *testing.T) {
	db := postgresFixture(t)
	raw, _, first := hexletPublicFixture(t, "java-basics-free")
	_, _, second := hexletPublicFixture(t, "javascript-basics-free")
	raw = bytes.Replace(raw, []byte(`content="`), []byte(`content="Программа предназначена для начинающих без опыта программирования. `), 1)
	ctx := context.Background()
	if err := db.Import(ctx, catalog.Dataset{Domains: []string{"ru.hexlet.io"}, Courses: []catalog.Course{first, second}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	calls := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != first.Source {
			t.Fatal("unselected source/probe requested", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw))}, nil
	})}
	model := &publicNetologyModel{}
	service := AIService{DB: db, Config: AIConfig{Model: "test", BatchSize: 40, DailyLimit: 2, CourseIDs: []string{first.ID}}, Model: model, Client: client}
	result, err := service.Run(ctx)
	if err != nil || result.Attempted != 1 || result.Queued != 1 || calls != 1 || model.calls != 2 {
		t.Fatal(result, calls, model.calls, err)
	}
	result, err = service.Run(ctx)
	if err != nil || result.Attempted != 0 || calls != 1 || model.calls != 2 {
		t.Fatal("targeted run bypassed cooldown", result, err)
	}
	var other int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_ai_sources WHERE course_id=$1", second.ID).Scan(&other); err != nil || other != 0 {
		t.Fatal("unselected state changed", other, err)
	}
	// A changed source misses cache but cannot reset the common daily budget.
	if _, err := db.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now()"); err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("Программа предназначена"), []byte("Эта программа предназначена"), 1)
	result, err = service.Run(ctx)
	if err != nil || result.Queued != 0 || model.calls != 2 {
		t.Fatal("shared budget bypassed", result, model.calls, err)
	}
	service.Config.CourseIDs = []string{first.ID, "missing"}
	previousCalls := calls
	if _, err := service.Run(ctx); err == nil || calls != previousCalls {
		t.Fatal("invalid selection made source/model requests", err)
	}
}
