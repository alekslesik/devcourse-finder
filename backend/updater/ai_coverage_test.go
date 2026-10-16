package updater

import (
	"bytes"
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestAIExpansionCoverageAndSharedBudget(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	coverage, e := aiCoverage(ctx, db)
	if e != nil || len(coverage) != 13 {
		t.Fatal(coverage, e)
	}
	for _, v := range coverage {
		if v.Eligible != 0 || v.Verified != 0 {
			t.Fatal(v)
		}
	}
	// Separate providers/engines still contend for the same two-request reservation.
	first := AIEngine{DB: db, DailyLimit: 2}
	second := AIEngine{DB: db, DailyLimit: 2}
	if e = first.reserve(ctx); e != nil {
		t.Fatal(e)
	}
	if e = second.reserve(ctx); e != ErrAIBudget {
		t.Fatal("providers split budget", e)
	}
}

func TestAISourceFailureDoesNotBlockAnotherProvider(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	raw, c := skyFixture(t, "python")
	raw = aiBoundFixtureText(raw, c)
	r, o, e := collectSkypro(raw, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	nraw, nc, nr, _ := aiNetologyFixture(t)
	nr.Profile = nil
	r.Profile = nil
	r.Audience = []string{"unknown"}
	r.Goals = []string{"unknown"}
	r.Direction = "unknown"
	if e = db.Import(ctx, catalog.Dataset{Domains: []string{"sky.pro", "netology.ru"}, Courses: []catalog.Course{nr, r}}, "operator", false); e != nil {
		t.Fatal(e)
	}
	_, e = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id,skypro_product_id) VALUES($1,$2,$3,$4,$5,$6)`, c.Adapter, c.ExternalID, c.URL, r.ID, r.Offers[0].ID, o.SkyproProductID)
	if e != nil {
		t.Fatal(e)
	}
	client := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		data := raw
		switch req.URL.String() {
		case candidateEndpoint(c):
		case candidateEndpoint(nc):
			data = nraw
		default:
			t.Fatal("unexpected URL", req.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}
	model := &boundAIModel{}
	s := AIService{DB: db, Client: client, Model: model, Config: AIConfig{Model: "fake", VerifyModel: "fake-review", BatchSize: 2, DailyLimit: 2}}
	result, e := s.Run(ctx)
	if e != nil || result.Attempted != 2 || result.Failed != 1 || result.Queued != 1 || model.calls != 2 {
		t.Fatal(result, e, model.calls)
	}
	var status bytes.Buffer
	if e = AIStatus(ctx, db, &status); e != nil {
		t.Fatal(e)
	}
	var header struct {
		Coverage []aiProviderCoverage `json:"provider_coverage"`
	}
	if e = json.Unmarshal(bytes.Split(status.Bytes(), []byte("\n"))[0], &header); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, v := range header.Coverage {
		if v.Provider == "skypro" {
			found = true
			if v.Eligible != 1 || v.Mapped != 1 || v.Attempted != 1 {
				t.Fatal(v)
			}
		}
	}
	if !found {
		t.Fatal("coverage missing")
	}
}
