package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
)

func acceptanceCourse(id string, now time.Time) catalog.Course {
	return catalog.Course{
		ID: id, Slug: id, Title: id, Provider: "School", Language: "go", Direction: "backend",
		Summary: "Full programming course", Audience: []string{"switch"}, Goals: []string{"switch"},
		Topics: []string{"Go"}, Source: "https://example.com/course", CheckedAt: now, Status: "published", Demo: true,
		Offers: []catalog.Offer{{ID: id + "-full", Name: "Full course", Price: value(int64(2000000)),
			PriceKind: "exact", PriceCheckedAt: now, Schedule: "flexible", Enrollment: "continuous", URL: "https://example.com/enroll"}},
	}
}

// Exercise validated publication, PostgreSQL, the production cache, query parser,
// search and wire JSON together. Each test owns a separate disposable schema.
func acceptanceSearch(t *testing.T, courses []catalog.Course) func(string) []catalog.Result {
	t.Helper()
	db := postgresHTTPFixture(t)
	if _, err := db.Pool.Exec(context.Background(), "TRUNCATE courses CASCADE"); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(catalog.Dataset{Domains: []string{"example.com"}, Courses: courses})
	if err != nil {
		t.Fatal(err)
	}
	valid, err := catalog.Decode(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Import(context.Background(), valid, "acceptance-test", false); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newHandler(store.NewCached(db)))
	t.Cleanup(server.Close)
	client := &http.Client{Timeout: 3 * time.Second}
	contract := newResponseContract(t)
	return func(query string) []catalog.Result {
		t.Helper()
		response := wireRequest(t, client, server.URL, "GET", "/api/v1/courses?page_size=48&"+query, "")
		contract.check(t, response, "/api/v1/courses", "GET", 200)
		var body struct {
			Items []catalog.Result `json:"items"`
			Total int              `json:"total"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Total != len(body.Items) {
			t.Fatalf("unexpected total %d for %d items", body.Total, len(body.Items))
		}
		return body.Items
	}
}

func acceptanceOffers(t *testing.T, results []catalog.Result, want ...string) {
	t.Helper()
	got := make([]string, 0, len(results))
	for _, result := range results {
		got = append(got, result.Offer.ID)
	}
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("offers %v, want %v", got, want)
	}
}

func TestAcceptanceAC02GoSwitchBudget(t *testing.T) {
	now := time.Now().UTC()
	valid := acceptanceCourse("matching", now)
	boundary := acceptanceCourse("budget-boundary", now)
	boundary.Offers[0].Price = value(int64(3000000))
	wrongLanguage := acceptanceCourse("python", now)
	wrongLanguage.Language = "python"
	wrongGoal := acceptanceCourse("first-job", now)
	wrongGoal.Goals = []string{"job"}
	wrongAudience := acceptanceCourse("beginner", now)
	wrongAudience.Audience = []string{"none"}
	expensive := acceptanceCourse("over-budget", now)
	expensive.Offers[0].Price = value(int64(3000001))
	search := acceptanceSearch(t, []catalog.Course{valid, boundary, wrongLanguage, wrongGoal, wrongAudience, expensive})
	query := "language=go&experience=switch&goal=switch&max=3000000"
	results := search(query)
	acceptanceOffers(t, results, "matching-full", "budget-boundary-full")
	for _, result := range results {
		if result.Course.Language != "go" || !slices.Contains(result.Course.Audience, "switch") || !slices.Contains(result.Course.Goals, "switch") || result.Price == nil || *result.Price > 3000000 {
			t.Fatalf("mismatched result: %+v", result)
		}
	}
	acceptanceOffers(t, search(query+"&min=3000000"), "budget-boundary-full")
}
