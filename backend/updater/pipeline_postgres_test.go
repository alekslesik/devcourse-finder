package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
)

func TestDiscoveryPublishesDistinctCoursesAutomaticallyAndIsolatesFailures(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	ctx := context.Background()
	config.Discovery = []Feed{{ID: "otus", Adapter: "otus", URL: "https://otus.ru/sitemap.xml"}, {ID: "yandex", Adapter: "yandex", URL: "https://practicum.yandex.ru/sitemap.xml"}}
	otus := string(fixtureHTML(t, "otus-python-schema.json"))
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		status := 200
		body := ""
		switch {
		case r.URL.Host == "practicum.yandex.ru":
			status = 404
		case r.URL.Path == "/sitemap.xml":
			body = `<urlset><url><loc>https://otus.ru/lessons/python-basic/</loc></url><url><loc>https://otus.ru/lessons/java-basic/</loc></url><url><loc>https://otus.ru/about</loc></url><url><loc>https://attacker.example/lessons/python</loc></url></urlset>`
		case r.URL.Path == "/lessons/python-basic/":
			body = otus
		case r.URL.Path == "/lessons/java-basic/":
			body = strings.ReplaceAll(strings.ReplaceAll(otus, "python-basic", "java-basic"), "Python", "Java")
		default:
			c := validStepik()
			if strings.HasSuffix(r.URL.Path, "58852") {
				c["id"] = 58852
				c["title"] = `"Поколение Python": курс для начинающих`
			}
			body = string(stepikJSON(c))
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	service := Service{DB: db, Config: config, Client: client}
	r, err := service.Run(ctx)
	if err != nil || r.Published != 4 || r.Status != "partial" {
		t.Fatalf("pipeline %#v %v", r, err)
	}
	courses, err := db.Load(ctx)
	if err != nil || len(courses) != 4 {
		t.Fatal("candidate didn't publish", len(courses), err)
	}
	var identities, count int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_identities").Scan(&identities)
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_candidates WHERE state='published'").Scan(&count)
	if identities != 2 || count != 2 {
		t.Fatal("missing queue state/mapping", identities, count)
	}
	// Repeated discovery sees the same URLs; a live detail refresh commits history
	// and invalidates caches without adding another course.
	cache := store.NewCached(db)
	old, err := cache.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db.Pool.Exec(ctx, "UPDATE catalog_candidates SET next_attempt_at=now()")
	r, err = service.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, err := cache.Load(ctx)
	if err != nil || len(after) != len(old) {
		t.Fatal("duplicate/cache failure", err)
	}
	var output strings.Builder
	if err = Status(ctx, db, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"provider":"OTUS"`) || !strings.Contains(output.String(), `"queue":true`) || !strings.Contains(output.String(), `"code":"feed_unavailable"`) {
		t.Fatal("coverage/source failures not reported")
	}
}
func TestGrowingQueueReachesHundredWithoutDuplicateIDs(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	config.Discovery = []Feed{{ID: "stepik", Adapter: "stepik", URL: "https://stepik.org/sitemap.xml"}}
	var xml strings.Builder
	xml.WriteString("<urlset>")
	for i := 1000; i < 1100; i++ {
		fmt.Fprintf(&xml, "<url><loc>https://stepik.org/course/%d/promo</loc></url>", i)
	}
	xml.WriteString("</urlset>")
	names := []string{"Go", "Python", "Java", "JavaScript"}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		body := xml.String()
		if r.URL.Path != "/sitemap.xml" {
			remote, err := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/api/courses/"))
			if err != nil {
				t.Fatal(err)
			}
			c := validStepik()
			c["id"] = remote
			c["title"] = names[remote%4] + " course"
			c["summary"] = "Полная учебная программа"
			c["lessons_count"] = 10
			c["total_units"] = 10
			c["language"] = "ru"
			if remote == 54403 {
				c["title"] = testSource.ExpectedTitle
			}
			if remote == 58852 {
				c["title"] = `"Поколение Python": курс для начинающих`
			}
			body = string(stepikJSON(c))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	service := Service{DB: db, Config: config, Client: client}
	for run := 0; run < 18; run++ {
		if _, err := service.Run(ctx); err != nil {
			t.Fatal(err)
		}
		// Simulate the next scheduled window for pending work; published records
		// retain their refresh schedule, exercising queue progress across runs.
		db.Pool.Exec(ctx, "UPDATE catalog_candidates SET next_attempt_at=now() WHERE state='pending'")
	}
	var count, languages int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*),count(DISTINCT language) FROM courses WHERE status='published'").Scan(&count, &languages); err != nil || count != 102 || languages != 4 {
		t.Fatalf("coverage %d/%d %v", count, languages, err)
	}
	var mappings int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_identities").Scan(&mappings)
	if mappings != 100 {
		t.Fatal("duplicates/missing mappings", mappings)
	}
}
func TestPublishedCourseClosureAndManualURLProtection(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	candidate := Candidate{Adapter: "otus", ExternalID: "python-basic", URL: "https://otus.ru/lessons/python-basic"}
	record, o, err := collectCandidate(fixtureHTML(t, "otus-python-schema.json"), candidate, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	service := Service{DB: db, Config: config}
	if code, n, err := service.publishDiscovered(ctx, candidate, record, o, ""); err != nil || n != 1 || code != "verified" {
		t.Fatal(code, n, err)
	}
	o.Enrollment = "closed"
	if code, n, err := service.publishDiscovered(ctx, candidate, record, o, ""); err != nil || n != 0 || code != "pending_confirmation" {
		t.Fatal("closure not held", code, n, err)
	}
	db.Pool.Exec(ctx, "UPDATE updater_candidates SET observed_at=now()-interval '12 hours'")
	if code, n, err := service.publishDiscovered(ctx, candidate, record, o, ""); err != nil || n != 1 || code != "verified" {
		t.Fatal("confirmed closure ignored", code, n, err)
	}
	old, _ := db.Load(ctx)
	if old[0].Offers[0].Enrollment != "closed" {
		t.Fatal("closed course still open")
	}
	old[0].Offers[0].URL = "https://otus.ru/lessons/another-course"
	if err := db.Import(ctx, catalog.Dataset{Domains: []string{"otus.ru"}, Courses: old}, "operator", false); err != nil {
		t.Fatal(err)
	}
	o.Enrollment = "open"
	if code, n, err := service.publishDiscovered(ctx, candidate, record, o, ""); err != nil || n != 0 || code != "protected_identity" {
		t.Fatal("manual URL overwritten", code, n, err)
	}
}
func TestRecordedSourceFilesArePublicContractOnly(t *testing.T) {
	for _, name := range []string{"otus-python-schema.json", "yandex-python-schema.json", "stepik-go.json"} {
		b, err := os.ReadFile("testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if json.Unmarshal(b, &v) != nil {
			t.Fatal("invalid fixture")
		}
		if strings.Contains(string(b), "lti_secret") {
			t.Fatal("unrelated sensitive fields in fixture")
		}
	}
}

func TestNewClosedCourseCannotReserveIdentityOrPublish(t *testing.T) {
	db := postgresFixture(t)
	candidate := Candidate{Adapter: "otus", ExternalID: "python-basic", URL: "https://otus.ru/lessons/python-basic"}
	record, observation, err := collectCandidate(fixtureHTML(t, "otus-python-schema.json"), candidate, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	observation.Enrollment = "closed"
	service := Service{DB: db, Config: productionConfig(t)}
	code, n, err := service.publishDiscovered(context.Background(), candidate, record, observation, "")
	if err != nil || n != 0 || code != "insufficient_initial_evidence" {
		t.Fatal(code, n, err)
	}
	var count int
	if err = db.Pool.QueryRow(context.Background(), "SELECT count(*) FROM catalog_identities").Scan(&count); err != nil || count != 0 {
		t.Fatal("unpublished identity leaked", count, err)
	}
}

func TestFailedDiscoveredReadBreaksClosureConfirmation(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	candidate := Candidate{Adapter: "otus", ExternalID: "python-basic", URL: "https://otus.ru/lessons/python-basic"}
	record, observation, err := collectCandidate(fixtureHTML(t, "otus-python-schema.json"), candidate, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	config := productionConfig(t)
	config.Discovery = []Feed{{ID: "otus", Adapter: "otus", URL: "https://otus.ru/sitemap.xml"}}
	service := Service{DB: db, Config: config}
	if _, n, err := service.publishDiscovered(ctx, candidate, record, observation, ""); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	observation.Enrollment = "closed"
	if code, _, err := service.publishDiscovered(ctx, candidate, record, observation, ""); err != nil || code != "pending_confirmation" {
		t.Fatal(code, err)
	}
	if _, err = db.Pool.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state) VALUES('otus','python-basic','https://otus.ru/lessons/python-basic','otus','published')`); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}, Request: r}, nil
	})}
	if stats, err := service.processBatch(ctx, client, service.publishDiscovered); err != nil || stats.Failed != 1 {
		t.Fatal(stats, err)
	}
	var confirmations int
	if err = db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&confirmations); err != nil || confirmations != 0 {
		t.Fatal("failed read retained confirmation", confirmations, err)
	}
}
