package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
)

func postgresHTTPFixture(t *testing.T) *store.DB {
	t.Helper()
	connection := os.Getenv("TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("TEST_DATABASE_URL is required for HTTP tests with PostgreSQL")
	}
	parsed, err := url.Parse(connection)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("HTTP integration tests require a dedicated database ending in _test")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	name := "http_test_" + newID()
	schema := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Pool.Close()
		t.Fatal(err)
	}
	var db *store.DB
	t.Cleanup(func() {
		if db != nil {
			db.Pool.Close()
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Pool.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Errorf("cleanup test schema: %v", err)
		}
		admin.Pool.Close()
	})
	query := parsed.Query()
	query.Set("search_path", name)
	parsed.RawQuery = query.Encode()
	db, err = store.Open(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	repository := httpFixture()
	data := catalog.Dataset{Courses: repository.courses, Domains: repository.domains}
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	valid, err := catalog.Decode(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Import(ctx, valid, "http-integration-test", false); err != nil {
		t.Fatal(err)
	}
	return db
}
func wireRequest(t *testing.T, client *http.Client, origin, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req, err := http.NewRequest(method, origin+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "untrusted-client-id")
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	recorded := httptest.NewRecorder()
	for key, values := range response.Header {
		recorded.Header()[key] = values
	}
	recorded.WriteHeader(response.StatusCode)
	recorded.Body.Write(raw)
	return recorded
}
func TestHTTPWithPostgresAndFailedAnalytics(t *testing.T) {
	db := postgresHTTPFixture(t)
	server := httptest.NewServer(newHandler(store.NewCached(db)))
	defer server.Close()
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	contract := newResponseContract(t)
	ctx := context.Background()
	send := func(method, path, body string) *httptest.ResponseRecorder {
		return wireRequest(t, client, server.URL, method, path, body)
	}
	contract.check(t, send("GET", "/health/ready", ""), "/health/ready", "GET", 200)
	body := contract.check(t, send("GET", "/api/v1/courses?max=5000000&support=review", ""), "/api/v1/courses", "GET", 200).(map[string]any)
	items := body["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["offer"].(map[string]any)["id"] != "review" {
		t.Fatal("PostgreSQL search did not select the matching tariff")
	}
	for _, slug := range []string{"draft", "archived"} {
		contract.check(t, send("GET", "/api/v1/courses/"+slug, ""), "/api/v1/courses/{slug}", "GET", 404)
	}
	comparison := contract.check(t, send("GET", "/api/v1/compare?offer_ids=self,closed,archived-self", ""), "/api/v1/compare", "GET", 200).([]any)
	if len(comparison) != 3 || comparison[1].(map[string]any)["reason"] != "closed" || comparison[2].(map[string]any)["unavailable"] != true {
		t.Fatal("unavailable comparisons lost their positions")
	}
	for i := 0; i < 2; i++ {
		contract.check(t, send("POST", "/api/v1/events", `{"id":"event-0001","kind":"view","course_id":"go-course"}`), "/api/v1/events", "POST", 204)
	}
	var count int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE id='event-0001'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("event is not deduplicated: %d, %v", count, err)
	}
	if err := db.Pool.QueryRow(ctx, "SELECT count FROM daily_stats WHERE kind='view' AND course_id='go-course'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("aggregate double-counted an event: %d, %v", count, err)
	}
	updated := httpFixture()
	updated.courses[0].Offers[1].Price = value(int64(2500000))
	if err := db.Import(ctx, catalog.Dataset{Courses: updated.courses, Domains: updated.domains}, "http-integration-test", false); err != nil {
		t.Fatal(err)
	}
	body = contract.check(t, send("GET", "/api/v1/courses?max=3000000&support=review", ""), "/api/v1/courses", "GET", 200).(map[string]any)
	if body["total"] != float64(1) {
		t.Fatal("HTTP search retained an obsolete price")
	}
	response := send("GET", "/out/self?url=https://attacker.example", "")
	contract.check(t, response, "/out/{offer_id}", "GET", 302)
	if response.Header().Get("Location") != "https://example.com/self" {
		t.Fatal("URL parameter changed the redirect target")
	}
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM events WHERE kind='outbound'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("outbound click not recorded: %d, %v", count, err)
	}
	// Fail actual analytics SQL in this isolated schema while catalog SQL still works.
	if _, err := db.Pool.Exec(ctx, "DROP TABLE events"); err != nil {
		t.Fatal(err)
	}
	contract.check(t, send("POST", "/api/v1/events", `{"id":"event-0002","kind":"view"}`), "/api/v1/events", "POST", 503)
	response = send("GET", "/out/self", "")
	contract.check(t, response, "/out/{offer_id}", "GET", 302)
	if response.Header().Get("Location") != "https://example.com/self" {
		t.Fatal("SQL analytics failure prevented redirect")
	}
	db.Pool.Close()
	contract.check(t, send("GET", "/api/v1/courses", ""), "/api/v1/courses", "GET", 503)
	contract.check(t, send("GET", "/health/ready", ""), "/health/ready", "GET", 503)
}
