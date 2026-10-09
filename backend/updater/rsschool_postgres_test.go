package updater

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func rsCurrent(b []byte, start, end time.Time) []byte {
	s := string(b)
	for _, pair := range [][2]string{{"2026-09-06T00:00:00.000Z", start.Format(time.RFC3339Nano)}, {"2026-09-27T23:59:59.999Z", end.Format(time.RFC3339Nano)}, {"Sep 06, 2026", start.Format("Jan 02, 2006")}, {"Sep 27, 2026", end.Format("Jan 02, 2006")}, {"js-stage1-2026q3", "js-stage1-" + start.Format("2006") + "q" + strconv.Itoa((int(start.Month())-1)/3+1)}} {
		s = strings.ReplaceAll(s, pair[0], pair[1])
	}
	return []byte(s)
}
func TestRSSchoolPublisherConfirmsClosureAcrossFreshnessLeases(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: "rsschool-catalog", Adapter: "rsschool", URL: "https://rs.school/courses", Kind: "catalog"}}
	original, indexOriginal, c := rsFixture(t, "javascript")
	now := time.Now().UTC()
	b := rsCurrent(original, now.Add(5*24*time.Hour), now.Add(20*24*time.Hour))
	index := rsCurrent(indexOriginal, now.Add(5*24*time.Hour), now.Add(20*24*time.Hour))
	catalogReads := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		body := b
		if r.URL.String() == "https://rs.school/courses" {
			body = index
			catalogReads++
		} else if r.URL.String() != c.URL {
			t.Fatal(r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}, Request: r}, nil
	})}
	_, err := db.Pool.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id) VALUES($1,$2,$3,$4)`, c.Adapter, c.ExternalID, c.URL, c.FeedID)
	if err != nil {
		t.Fatal(err)
	}
	collector := Service{DB: db, Config: config, Worker: "pages"}
	publisher := Service{DB: db, Config: config, Worker: "publisher"}
	run := func() {
		t.Helper()
		_, err := db.Pool.Exec(ctx, "UPDATE catalog_candidates SET next_attempt_at=now()")
		if err != nil {
			t.Fatal(err)
		}
		stats, err := collector.processBatch(ctx, client, collector.publishDiscovered)
		if err != nil || stats.Queued != 1 || stats.Failed != 0 {
			t.Fatal(stats, err)
		}
		if _, err = publisher.Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	run()
	courses, _ := db.Load(ctx)
	if len(courses) != 1 || !courses[0].Offers[0].Free || courses[0].Offers[0].Enrollment != "open" {
		t.Fatal(courses)
	}
	b = rsCurrent(original, now.Add(-30*24*time.Hour), now.Add(-9*24*time.Hour))
	index = rsCurrent(indexOriginal, now.Add(-30*24*time.Hour), now.Add(-9*24*time.Hour))
	run()
	courses, _ = db.Load(ctx)
	if courses[0].Offers[0].Enrollment != "open" {
		t.Fatal("unconfirmed closure published")
	}
	_, err = db.Pool.Exec(ctx, "UPDATE updater_candidates SET observed_at=now()-interval '7 hours'")
	if err != nil {
		t.Fatal(err)
	}
	run()
	courses, _ = db.Load(ctx)
	if courses[0].Offers[0].Enrollment != "closed" || catalogReads != 3 {
		t.Fatal("closure not confirmed across leases", courses, catalogReads)
	}
}
func TestRSSchoolDoesNotPublishClosedInitialCohort(t *testing.T) {
	db := postgresFixture(t)
	b, index, c := rsFixture(t, "javascript")
	r, o, err := collectRSSchool(b, index, c, netologyNow())
	if err != nil {
		t.Fatal(err)
	}
	service := Service{DB: db}
	code, n, err := service.publishDiscovered(context.Background(), c, r, o, fingerprint(b))
	if err != nil || n != 0 || code != "insufficient_initial_evidence" {
		t.Fatal(code, n, err)
	}
}

func TestRSSchoolFailedCatalogBlocksWholeBatch(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: "rsschool-catalog", Adapter: "rsschool", URL: "https://rs.school/courses", Kind: "catalog"}}
	for _, slug := range []string{"javascript", "nodejs"} {
		_, err := db.Pool.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id) VALUES('rsschool',$1,$2,'rsschool-catalog')`, slug, "https://rs.school/courses/"+slug)
		if err != nil {
			t.Fatal(err)
		}
	}
	b, _, _ := rsFixture(t, "javascript")
	reads := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		status := 200
		body := string(b)
		if r.URL.String() == "https://rs.school/courses" {
			reads++
			status = 403
			body = "unavailable"
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	service := Service{DB: db, Config: config, Worker: "pages"}
	stats, err := service.processBatch(ctx, client, service.publishDiscovered)
	if err != nil || stats.Failed != 2 || stats.Queued != 2 || stats.Published != 0 || reads != 1 {
		t.Fatal(stats, reads, err)
	}
}
