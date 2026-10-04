package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
)

func remainingHTTP(t *testing.T, courses []catalog.Course) (*store.DB, *httptest.Server) {
	t.Helper()
	db := postgresHTTPFixture(t)
	if courses != nil {
		if _, err := db.Pool.Exec(context.Background(), "TRUNCATE courses CASCADE"); err != nil {
			t.Fatal(err)
		}
		publishRemaining(t, db, courses)
	}
	server := httptest.NewServer(newHandler(store.NewCached(db)))
	t.Cleanup(server.Close)
	return db, server
}

func publishRemaining(t *testing.T, db *store.DB, courses []catalog.Course) {
	t.Helper()
	raw, err := json.Marshal(catalog.Dataset{Courses: courses, Domains: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	valid, err := catalog.Decode(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Import(context.Background(), valid, "acceptance-test", false); err != nil {
		t.Fatal(err)
	}
}

func TestAcceptanceAC09URLUsesCurrentDatabase(t *testing.T) {
	now := time.Now().UTC()
	courses := []catalog.Course{acceptanceCourse("first", now), acceptanceCourse("second", now), acceptanceCourse("third", now)}
	for i := range courses {
		courses[i].Offers[0].Price = value(int64(i+1) * 1000000)
	}
	db, server := remainingHTTP(t, courses)
	contract := newResponseContract(t)
	client := &http.Client{Timeout: 3 * time.Second}
	query := "/api/v1/courses?language=go&experience=switch&goal=switch&sort=price_asc&page=2&page_size=1"
	check := func(want string) {
		t.Helper()
		body := contract.check(t, wireRequest(t, client, server.URL, "GET", query, ""), "/api/v1/courses", "GET", 200).(map[string]any)
		items := body["items"].([]any)
		if body["page"] != float64(2) || body["page_size"] != float64(1) || body["total"] != float64(3) || len(items) != 1 || items[0].(map[string]any)["course"].(map[string]any)["id"] != want {
			t.Fatalf("URL did not restore current page/sort/filter: %+v", body)
		}
	}
	check("second")
	courses[1].Offers[0].Price = value(int64(5000000))
	publishRemaining(t, db, courses)
	check("third")
}

func TestAcceptanceAC12RedirectSurvivesAnalyticsFailureAndIgnoresURL(t *testing.T) {
	db, server := remainingHTTP(t, nil)
	if _, err := db.Pool.Exec(context.Background(), "DROP TABLE events"); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	contract := newResponseContract(t)
	for _, query := range []string{"", "?url=https://attacker.example", "?redirect=https://attacker.example&target=//attacker.example"} {
		response := wireRequest(t, client, server.URL, "GET", "/out/self"+query, "")
		contract.check(t, response, "/out/{offer_id}", "GET", 302)
		if response.Header().Get("Location") != "https://example.com/self" {
			t.Fatal("redirect did not use the approved official URL")
		}
	}
	contract.check(t, wireRequest(t, client, server.URL, "POST", "/api/v1/events", `{"id":"ac12-event","kind":"view"}`), "/api/v1/events", "POST", 503)
}

func TestAcceptanceAC13OfficialURLsAndUnaffectedOrdering(t *testing.T) {
	now := time.Now().UTC()
	courses := []catalog.Course{acceptanceCourse("official-first", now), acceptanceCourse("official-second", now)}
	courses[0].Offers[0].Price = value(int64(1000000))
	courses[1].Offers[0].Price = value(int64(2000000))
	_, server := remainingHTTP(t, courses)
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	contract := newResponseContract(t)
	var first string
	for _, suffix := range []string{"", "&affiliate=https://attacker.example&ref=paid&url=https://attacker.example"} {
		body := contract.check(t, wireRequest(t, client, server.URL, "GET", "/api/v1/courses?sort=price_asc"+suffix, ""), "/api/v1/courses", "GET", 200).(map[string]any)
		raw, _ := json.Marshal(body["items"])
		if first == "" {
			first = string(raw)
		} else if first != string(raw) {
			t.Fatal("referral parameters changed the search order or data")
		}
		items := body["items"].([]any)
		if len(items) != 2 || items[0].(map[string]any)["course"].(map[string]any)["id"] != "official-first" {
			t.Fatal("wrong organic order")
		}
	}
	response := wireRequest(t, client, server.URL, "GET", "/out/"+courses[0].Offers[0].ID+"?affiliate=https://attacker.example", "")
	contract.check(t, response, "/out/{offer_id}", "GET", 302)
	if response.Header().Get("Location") != courses[0].Offers[0].URL {
		t.Fatal("official URL fallback missing")
	}
}

func TestAcceptanceAC14RepeatedCLIImportDoesNotDuplicate(t *testing.T) {
	db := postgresHTTPFixture(t)
	source := httpFixture()
	file := cliFile(t, catalog.Dataset{Courses: source.courses, Domains: source.domains})
	var previous string
	for i := 0; i < 2; i++ {
		result := cliProcess(t, db.Pool.Config().ConnString(), nil, "catalog", "import", file)
		if result.exit != 0 {
			t.Fatalf("CLI import failed: %s", result.stderr)
		}
		loaded, err := db.Load(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(loaded)
		if i > 0 && previous != string(raw) {
			t.Fatal("repeated import changed catalog")
		}
		previous = string(raw)
		var courses, offers int
		if err := db.Pool.QueryRow(context.Background(), "SELECT (SELECT count(*) FROM courses),(SELECT count(*) FROM offers)").Scan(&courses, &offers); err != nil {
			t.Fatal(err)
		}
		if courses != 3 || offers != 12 {
			t.Fatalf("duplicate rows: %d courses, %d offers", courses, offers)
		}
	}
}
