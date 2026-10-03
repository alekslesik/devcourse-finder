package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type testRepository struct {
	courses                    []catalog.Course
	domains                    []string
	loadErr, pingErr, eventErr error
	eventHook                  func(context.Context) error
	eventCalls                 int
}

func (r *testRepository) Load(context.Context) ([]catalog.Course, error) { return r.courses, r.loadErr }
func (r *testRepository) Domains(context.Context) []string               { return r.domains }
func (r *testRepository) Ping(context.Context) error                     { return r.pingErr }
func (r *testRepository) Event(ctx context.Context, _, _, _, _, _ string, _ int) error {
	r.eventCalls++
	if r.eventHook != nil {
		return r.eventHook(ctx)
	}
	return r.eventErr
}
func value[T any](v T) *T { return &v }
func httpFixture() *testRepository {
	now := time.Now().UTC()
	course := catalog.Course{ID: "go-course", Slug: "go-course", Title: "Go course", Provider: "School", Language: "go", Direction: "backend", Summary: "Practice Go", Audience: []string{"switch"}, Goals: []string{"switch"}, Topics: []string{"Go"}, Source: "https://example.com/course", CheckedAt: now, Status: "published", Offers: []catalog.Offer{
		{ID: "self", Name: "Self", Price: value(int64(2000000)), PriceKind: "exact", PriceCheckedAt: now, Weeks: value(8), Schedule: "flexible", Enrollment: "continuous", URL: "https://example.com/self"},
		{ID: "review", Name: "Review", Price: value(int64(4000000)), PriceKind: "exact", PriceCheckedAt: now, Hours: value(8), Weeks: value(12), Review: true, Schedule: "scheduled", Enrollment: "open", URL: "https://example.com/review"},
		{ID: "closed", Name: "Closed", Price: value(int64(1000000)), PriceKind: "exact", PriceCheckedAt: now, Schedule: "flexible", Enrollment: "closed", URL: "https://example.com/closed"},
		{ID: "unknown", Name: "Unknown", PriceKind: "unknown", Schedule: "unknown", Enrollment: "unknown", URL: "https://example.com/unknown"},
	}}
	courses := []catalog.Course{course}
	for _, status := range []string{"draft", "archived"} {
		private := course
		private.ID = status
		private.Slug = status
		private.Status = status
		private.Offers = append([]catalog.Offer(nil), course.Offers...)
		for i := range private.Offers {
			private.Offers[i].ID = status + "-" + private.Offers[i].ID
		}
		courses = append(courses, private)
	}
	return &testRepository{courses: courses, domains: []string{"example.com"}}
}
func request(handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", "untrusted-client-id")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

type responseContract struct{ compiler *jsonschema.Compiler }

func newResponseContract(t *testing.T) *responseContract {
	t.Helper()
	raw, err := os.ReadFile("../docs/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc any
	if err = json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err = compiler.AddResource("https://contract.example/openapi.json", doc); err != nil {
		t.Fatal(err)
	}
	return &responseContract{compiler: compiler}
}
func (c *responseContract) check(t *testing.T, response *httptest.ResponseRecorder, path, method string, status int) any {
	t.Helper()
	if response.Code != status {
		t.Fatalf("%s %s: status %d, want %d; body %s", method, path, response.Code, status, response.Body.String())
	}
	id := response.Header().Get("X-Request-ID")
	if len(id) != 32 || id == "untrusted-client-id" {
		t.Fatalf("invalid server request_id %q", id)
	}
	if status == 204 {
		if response.Body.Len() != 0 {
			t.Fatal("204 response must not contain a body")
		}
		return nil
	}
	if status == 302 {
		if response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("redirect must not be cached")
		}
		return nil
	}
	if !strings.HasPrefix(response.Header().Get("Content-Type"), "application/json") {
		t.Fatal("response is not JSON")
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("response must not be cached")
	}
	pointer := strings.ReplaceAll(strings.ReplaceAll(path, "~", "~0"), "/", "~1")
	schema, err := c.compiler.Compile(fmt.Sprintf("https://contract.example/openapi.json#/paths/%s/%s/responses/%d/content/application~1json/schema", pointer, strings.ToLower(method), status))
	if err != nil {
		t.Fatal(err)
	}
	var body any
	if err = json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(body); err != nil {
		t.Fatalf("response violates OpenAPI: %v", err)
	}
	if status >= 400 {
		errorBody := body.(map[string]any)
		if errorBody["request_id"] != id {
			t.Fatal("error request_id differs from header")
		}
		if strings.Contains(response.Body.String(), "private-database-details") {
			t.Fatal("internal database details leaked")
		}
	}
	return body
}
func TestHTTPPublicRoutesAndContract(t *testing.T) {
	repository := httpFixture()
	handler := newHandler(repository)
	contract := newResponseContract(t)
	for _, path := range []string{"/health/live", "/health/ready", "/api/v1/catalog/options"} {
		contract.check(t, request(handler, "GET", path, ""), path, "GET", 200)
	}
	response := request(handler, "GET", "/api/v1/courses?language=go&experience=switch&goal=switch&max=5000000&support=review&hours=10", "")
	body := contract.check(t, response, "/api/v1/courses", "GET", 200).(map[string]any)
	items := body["items"].([]any)
	if body["total"] != float64(1) || len(items) != 1 || items[0].(map[string]any)["offer"].(map[string]any)["id"] != "review" {
		t.Fatalf("wrong matching offer: %v", body)
	}
	body = contract.check(t, request(handler, "GET", "/api/v1/courses?max=3000000&support=review", ""), "/api/v1/courses", "GET", 200).(map[string]any)
	if body["total"] != float64(0) || len(body["items"].([]any)) != 0 {
		t.Fatal("combined price and support from different tariffs")
	}
	body = contract.check(t, request(handler, "GET", "/api/v1/courses?page=2&page_size=1&ignored=value", ""), "/api/v1/courses", "GET", 200).(map[string]any)
	if body["total"] != float64(1) || len(body["items"].([]any)) != 0 || body["page"] != float64(2) {
		t.Fatalf("wrong pagination: %v", body)
	}
	detail := contract.check(t, request(handler, "GET", "/api/v1/courses/go-course", ""), "/api/v1/courses/{slug}", "GET", 200).(map[string]any)
	if len(detail["offers"].([]any)) != 4 {
		t.Fatal("detail does not contain the program tariffs")
	}
	for _, slug := range []string{"missing", "draft", "archived"} {
		contract.check(t, request(handler, "GET", "/api/v1/courses/"+slug, ""), "/api/v1/courses/{slug}", "GET", 404)
	}
	// Four raw IDs, but only three distinct IDs, retain their original order.
	compared := contract.check(t, request(handler, "GET", "/api/v1/compare?offer_ids=review,closed,missing,review", ""), "/api/v1/compare", "GET", 200).([]any)
	if len(compared) != 3 || compared[0].(map[string]any)["offer"].(map[string]any)["id"] != "review" || compared[1].(map[string]any)["reason"] != "closed" || compared[2].(map[string]any)["id"] != "missing" {
		t.Fatalf("wrong comparison order or closed status: %v", compared)
	}
	for _, id := range []string{"draft-self", "archived-self"} {
		unavailable := contract.check(t, request(handler, "GET", "/api/v1/compare?offer_ids="+id, ""), "/api/v1/compare", "GET", 200).([]any)
		if unavailable[0].(map[string]any)["unavailable"] != true {
			t.Fatal("unpublished program exposed through comparison")
		}
	}
	contract.check(t, request(handler, "POST", "/api/v1/events", `{"id":"event-0001","kind":"search","language":"go","goal":"switch","total":1}`), "/api/v1/events", "POST", 204)
	if repository.eventCalls != 1 {
		t.Fatal("event was not recorded")
	}
}
func TestHTTPInvalidParametersAndBodies(t *testing.T) {
	handler := newHandler(httpFixture())
	contract := newResponseContract(t)
	for _, query := range []string{"language=rust", "direction=invalid", "experience=senior", "goal=invalid", "budget=invalid", "support=invalid", "schedule=invalid", "sort=commission", "min=-1", "max=10000000001", "min=20&max=10", "hours=0", "hours=169", "page=0", "page=1000001", "page_size=0", "page_size=49", "include_free=1", "include_closed=yes", "max=abc"} {
		t.Run(query, func(t *testing.T) {
			contract.check(t, request(handler, "GET", "/api/v1/courses?"+query, ""), "/api/v1/courses", "GET", 400)
		})
	}
	for _, ids := range []string{"", "self,,review", "self,review,closed,unknown"} {
		contract.check(t, request(handler, "GET", "/api/v1/compare?offer_ids="+ids, ""), "/api/v1/compare", "GET", 400)
	}
	for _, body := range []string{`{}`, `null`, `{"id":"short","kind":"view"}`, `{"id":"event-0001","kind":"buy"}`, `{"id":"event-0001","kind":"view","unknown":"value"}`, `{"id":"event-0001","kind":"search","total":-1}`, `{"id":"event-0001","kind":"view"} {}`, `{"id":"event-0001","kind":"view"} garbage`, strings.Repeat("x", 2049)} {
		contract.check(t, request(handler, "POST", "/api/v1/events", body), "/api/v1/events", "POST", 400)
	}
	response := request(handler, "POST", "/api/v1/courses", "")
	contract.check(t, response, "/api/v1/courses", "GET", 405)
	if !strings.Contains(response.Header().Get("Allow"), "GET") {
		t.Fatal("405 is missing Allow header")
	}
	response = request(handler, "GET", "/api/v1/unknown", "")
	body := contract.check(t, response, "/api/v1/courses/{slug}", "GET", 404).(map[string]any)
	if body["code"] != "not_found" {
		t.Fatal("router error is not normalized")
	}
}
func TestHTTPDatabaseFailuresRemainSafe(t *testing.T) {
	repository := httpFixture()
	repository.loadErr = errors.New("private-database-details")
	repository.pingErr = repository.loadErr
	handler := newHandler(repository)
	contract := newResponseContract(t)
	for _, route := range []struct{ target, path string }{{"/health/ready", "/health/ready"}, {"/api/v1/courses", "/api/v1/courses"}, {"/api/v1/courses/go-course", "/api/v1/courses/{slug}"}, {"/api/v1/compare?offer_ids=self", "/api/v1/compare"}, {"/out/self", "/out/{offer_id}"}} {
		contract.check(t, request(handler, "GET", route.target, ""), route.path, "GET", 503)
	}
	contract.check(t, request(handler, "GET", "/health/live", ""), "/health/live", "GET", 200)
	repository.eventErr = errors.New("private-database-details")
	contract.check(t, request(handler, "POST", "/api/v1/events", `{"id":"event-0001","kind":"view"}`), "/api/v1/events", "POST", 503)
}
func TestHTTPOutboundSafetyAndAnalyticsFailure(t *testing.T) {
	contract := newResponseContract(t)
	for _, failure := range []string{"error", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			repository := httpFixture()
			repository.eventErr = errors.New("private-database-details")
			if failure == "timeout" {
				repository.eventHook = func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
			}
			started := time.Now()
			response := request(newHandler(repository), "GET", "/out/self?url=https://attacker.example", "")
			contract.check(t, response, "/out/{offer_id}", "GET", 302)
			if response.Header().Get("Location") != "https://example.com/self" || repository.eventCalls != 1 {
				t.Fatal("redirect changed or analytics was not attempted")
			}
			if time.Since(started) > time.Second {
				t.Fatal("analytics blocked the redirect")
			}
		})
	}
	for _, id := range []string{"missing", "draft-self", "archived-self"} {
		contract.check(t, request(newHandler(httpFixture()), "GET", "/out/"+id, ""), "/out/{offer_id}", "GET", 404)
	}
	for _, target := range []string{"http://example.com/course", "javascript:alert(1)", "https://attacker.example", "https://example.com.attacker.example", "https://127.0.0.1", "https://localhost", "https://school.internal", "https://example.com:8443", "https://user:pass@example.com"} {
		repository := httpFixture()
		repository.courses[0].Offers[0].URL = target
		response := request(newHandler(repository), "GET", "/out/self", "")
		contract.check(t, response, "/out/{offer_id}", "GET", 404)
		if response.Header().Get("Location") != "" || repository.eventCalls != 0 {
			t.Fatal("unsafe redirect or click recorded")
		}
	}
	repository := httpFixture()
	repository.domains = nil
	contract.check(t, request(newHandler(repository), "GET", "/out/self", ""), "/out/{offer_id}", "GET", 404)
}
func TestHTTPEventsRateLimit(t *testing.T) {
	handler := newHandler(httpFixture())
	contract := newResponseContract(t)
	for i := 0; i < 1000; i++ {
		response := request(handler, "POST", "/api/v1/events", `{"id":"event-0001","kind":"view"}`)
		if response.Code == 429 {
			contract.check(t, response, "/api/v1/events", "POST", 429)
			return
		}
		if response.Code != 204 {
			t.Fatalf("unexpected status %d", response.Code)
		}
	}
	t.Fatal("event burst was not rate limited")
}
func TestEventLimiterRefill(t *testing.T) {
	now := time.Now()
	limiter := eventLimiter{tokens: 100, last: now}
	for i := 0; i < 100; i++ {
		if !limiter.allow(now) {
			t.Fatal("initial burst unexpectedly rejected")
		}
	}
	if limiter.allow(now) {
		t.Fatal("burst limit exceeded")
	}
	if !limiter.allow(now.Add(10*time.Millisecond)) || limiter.allow(now.Add(10*time.Millisecond)) {
		t.Fatal("refill does not match 100 events/second")
	}
}

func TestEventLimiterConcurrentBurst(t *testing.T) {
	now := time.Now()
	limiter := eventLimiter{tokens: 100, last: now}
	outcomes := make(chan bool, 200)
	for i := 0; i < 200; i++ {
		go func() { outcomes <- limiter.allow(now) }()
	}
	accepted := 0
	for i := 0; i < 200; i++ {
		if <-outcomes {
			accepted++
		}
	}
	if accepted != 100 {
		t.Fatalf("concurrent burst admitted %d events, want 100", accepted)
	}
}
