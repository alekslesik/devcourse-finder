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

func TestAcceptanceAC03OneTariffMustMeetBudgetAndReview(t *testing.T) {
	now := time.Now().UTC()
	course := acceptanceCourse("two-tariffs", now)
	review := course.Offers[0]
	review.ID = "human-review"
	review.Name = "Human code review"
	review.Price = value(int64(4000000))
	review.Review = true
	course.Offers = append(course.Offers, review)
	search := acceptanceSearch(t, []catalog.Course{course})
	acceptanceOffers(t, search("max=3000000&support=review"))
	// Both positive controls are necessary: neither budget nor support may simply
	// exclude the entire course. Raising the budget selects the actual review offer.
	acceptanceOffers(t, search("max=3000000"), "two-tariffs-full")
	results := search("max=4000000&support=review")
	acceptanceOffers(t, results, "human-review")
	if !results[0].Offer.Review || results[0].Price == nil || *results[0].Price != 4000000 {
		t.Fatalf("price/support belong to different offers: %+v", results[0])
	}
	acceptanceOffers(t, search("max=4000000&support=self"), "two-tariffs-full")
}

func TestAcceptanceAC04UnconfirmedPrices(t *testing.T) {
	now := time.Now().UTC()
	from := acceptanceCourse("starting-price", now)
	from.Offers[0].PriceKind = "from"
	unknown := acceptanceCourse("unknown-price", now)
	unknown.Offers[0].PriceKind = "unknown"
	unknown.Offers[0].Price = nil
	known := acceptanceCourse("confirmed-price", now)
	known.Offers[0].Price = value(int64(2500000))
	search := acceptanceSearch(t, []catalog.Course{from, unknown, known})
	for _, query := range []string{"max=3000000", "min=0", "min=1000000&max=3000000", "budget=paid&max=3000000"} {
		acceptanceOffers(t, search(query), "confirmed-price-full")
	}
	results := search("sort=price_asc")
	acceptanceOffers(t, results, "starting-price-full", "unknown-price-full", "confirmed-price-full")
	if results[0].Offer.ID != "confirmed-price-full" {
		t.Fatal("unconfirmed starting price sorted as a confirmed full price")
	}
	for _, result := range results[1:] {
		if result.Price != nil || result.Offer.PriceKind == "exact" {
			t.Fatalf("unconfirmed price represented as exact: %+v", result)
		}
	}
}

func TestAcceptanceAC05FreeMeansFullCourse(t *testing.T) {
	now := time.Now().UTC()
	free := acceptanceCourse("full-free-course", now)
	free.Offers[0].Free = true
	free.Offers[0].Price = value(int64(0))
	paid := acceptanceCourse("paid-with-free-intro", now)
	paid.Summary = "First lesson is free; the full course is paid"
	paid.Offers[0].Name = "Full course after a free introduction"
	unconfirmed := acceptanceCourse("unconfirmed-free", now)
	unconfirmed.Offers[0].Free = true
	unconfirmed.Offers[0].Price = value(int64(0))
	unconfirmed.Offers[0].PriceCheckedAt = now.Add(-31 * 24 * time.Hour)
	search := acceptanceSearch(t, []catalog.Course{free, paid, unconfirmed})
	results := search("budget=free")
	acceptanceOffers(t, results, "full-free-course-full")
	if !results[0].Offer.Free || results[0].Price == nil || *results[0].Price != 0 {
		t.Fatal("free result lacks a confirmed zero full-course price")
	}
	acceptanceOffers(t, search("budget=paid&max=3000000"), "paid-with-free-intro-full")
	acceptanceOffers(t, search("budget=paid&max=3000000&include_free=true"), "paid-with-free-intro-full", "full-free-course-full")
}

func TestAcceptanceAC06ExpiredAndStalePrices(t *testing.T) {
	now := time.Now().UTC()
	expired := acceptanceCourse("expired-only", now)
	expired.Offers[0].Price = value(int64(1000000))
	expired.Offers[0].ValidUntil = value(now.Add(-time.Hour))
	stale := acceptanceCourse("stale-only", now)
	stale.Offers[0].PriceCheckedAt = now.Add(-31 * 24 * time.Hour)
	base := acceptanceCourse("current-base", now)
	base.Offers[0].Price = value(int64(2500000))
	discount := base.Offers[0]
	discount.ID = "expired-discount"
	discount.Price = value(int64(1000000))
	discount.ValidUntil = value(now.Add(-time.Hour))
	base.Offers = append(base.Offers, discount)
	fresh := acceptanceCourse("checked-29-days", now)
	fresh.Offers[0].PriceCheckedAt = now.Add(-29 * 24 * time.Hour)
	search := acceptanceSearch(t, []catalog.Course{expired, stale, base, fresh})
	results := search("max=3000000")
	acceptanceOffers(t, results, "current-base-full", "checked-29-days-full")
	acceptanceOffers(t, search("max=1500000"))
	results = search("sort=price_asc")
	acceptanceOffers(t, results, "current-base-full", "checked-29-days-full", "expired-only-full", "stale-only-full")
	if results[0].Offer.ID != "checked-29-days-full" || results[1].Offer.ID != "current-base-full" {
		t.Fatal("expired or stale amount sorted ahead of current prices")
	}
	for _, result := range results[2:] {
		if result.Price != nil {
			t.Fatalf("obsolete amount is still effective: %+v", result)
		}
	}
}

func TestAcceptanceAC06ExpiryIsAppliedToCachedReads(t *testing.T) {
	now := time.Now().UTC()
	course := acceptanceCourse("expiring-price", now)
	deadline := now.Add(3 * time.Second)
	course.Offers[0].ValidUntil = &deadline
	search := acceptanceSearch(t, []catalog.Course{course})
	acceptanceOffers(t, search("max=3000000"), "expiring-price-full")
	// No reimport, cleanup job or catalog revision change occurs. The same cached
	// snapshot must stop passing the budget when wall-clock validity expires.
	for time.Now().Before(deadline.Add(3 * time.Second)) {
		if results := search("max=3000000"); len(results) == 0 {
			unrestricted := search("")
			acceptanceOffers(t, unrestricted, "expiring-price-full")
			if unrestricted[0].Price != nil {
				t.Fatal("unrestricted search retained the expired amount")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("cached catalog ignored price expiration")
}
