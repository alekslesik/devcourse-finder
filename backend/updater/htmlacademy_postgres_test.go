package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTMLAcademyDiscoveryQueuePublicationAndFailedRefresh(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	config.Sources = nil
	feed := Feed{ID: "htmlacademy-sitemap", Adapter: "htmlacademy", URL: "https://htmlacademy.ru/sitemap.xml"}
	config.Discovery = []Feed{feed}
	page, payload, c := academyFixture(t, "javascript")
	payment := academyEnvelope(payload)
	status := 200
	paymentReads := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		body := ""
		code := 200
		switch r.URL.String() {
		case feed.URL:
			body = `<sitemapindex><sitemap><loc>https://htmlacademy.ru/sitemap/sitemap_courses.xml</loc></sitemap><sitemap><loc>https://htmlacademy.ru/sitemap/sitemap_default.xml</loc></sitemap></sitemapindex>`
		case "https://htmlacademy.ru/sitemap/sitemap_default.xml":
			body = `<urlset><url><loc>https://htmlacademy.ru/intensive/javascript</loc></url><url><loc>https://htmlacademy.ru/intensive/javascript/reviews</loc></url></urlset>`
		case c.URL:
			body = string(page)
		case "https://htmlacademy.ru" + academyPaymentPath(c):
			body = string(payment)
			code = status
			paymentReads++
		default:
			t.Fatal("unexpected read", r.URL)
		}
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	collector := Service{DB: db, Config: config, Worker: "pages"}
	publisher := Service{DB: db, Config: config, Worker: "publisher"}
	if n, err := collector.discoverFeed(ctx, client, feed); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	stats, err := collector.processBatch(ctx, client, collector.publishDiscovered)
	if err != nil || stats.Queued != 1 || stats.Published != 0 || stats.Failed != 0 {
		t.Fatal(stats, err)
	}
	courses, err := db.Load(ctx)
	if err != nil || len(courses) != 0 {
		t.Fatal("collector published", courses, err)
	}
	var raw string
	if err := db.Pool.QueryRow(ctx, "SELECT payload::text FROM catalog_observations LIMIT 1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "token") || strings.Contains(raw, "paymentWays") || strings.Contains(raw, "amoCrm") || strings.Contains(raw, "uid") {
		t.Fatal("raw payment data persisted")
	}
	if _, err := publisher.Run(ctx); err != nil {
		t.Fatal(err)
	}
	courses, err = db.Load(ctx)
	if err != nil || len(courses) != 1 || courses[0].Offers[0].Price != nil || courses[0].Offers[0].Free || courses[0].Offers[0].Enrollment != "continuous" {
		t.Fatal(courses, err)
	}
	before, _ := json.Marshal(courses)
	status = 403
	if _, err := db.Pool.Exec(ctx, "UPDATE catalog_candidates SET next_attempt_at=now()"); err != nil {
		t.Fatal(err)
	}
	stats, err = collector.processBatch(ctx, client, collector.publishDiscovered)
	if err != nil || stats.Failed != 1 || stats.Queued != 1 {
		t.Fatal(stats, err)
	}
	if _, err := publisher.Run(ctx); err != nil {
		t.Fatal(err)
	}
	courses, _ = db.Load(ctx)
	after, _ := json.Marshal(courses)
	if string(before) != string(after) || paymentReads != 2 {
		t.Fatal("failed payment read changed catalog", paymentReads)
	}
}
func TestHTMLAcademyPublisherRejectsInventedTotalsAndStaleEvidence(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: "htmlacademy-sitemap", Adapter: "htmlacademy", URL: "https://htmlacademy.ru/sitemap.xml"}}
	b, p, c := academyFixture(t, "javascript")
	now := time.Now().UTC()
	r, o, err := collectHTMLAcademy(b, academyEnvelope(p), c, now)
	if err != nil {
		t.Fatal(err)
	}
	e := QueuedObservation{Version: 1, Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: now, Digest: fingerprint(b)}
	if err := e.validate(config); err != nil {
		t.Fatal(err)
	}
	total := int64(2400000)
	bad := e
	bad.Observation.Price = &total
	bad.Observation.PriceUnknown = false
	if bad.validate(config) == nil {
		t.Fatal("invented subscription total accepted")
	}
	bad = e
	bad.Record.Offers = append([]catalog.Offer(nil), e.Record.Offers...)
	bad.Record.Offers[0].Price = &total
	bad.Record.Offers[0].PriceKind = "exact"
	if bad.validate(config) == nil {
		t.Fatal("invented offer total accepted")
	}
	publisher := Service{DB: db, Config: config, Worker: "publisher"}
	old := now.Add(-27 * time.Hour)
	r, o, err = collectHTMLAcademy(b, academyEnvelope(p), c, old)
	if err != nil {
		t.Fatal(err)
	}
	stale := QueuedObservation{Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: old, Digest: fingerprint(b)}
	if err := publisher.enqueue(ctx, stale); err != nil {
		t.Fatal(err)
	}
	if _, err := publisher.Run(ctx); err != nil {
		t.Fatal(err)
	}
	courses, err := db.Load(ctx)
	if err != nil || len(courses) != 0 {
		t.Fatal("expired evidence published", courses, err)
	}
}
