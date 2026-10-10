package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestBudgetSkipStillProcessesCheaperProvider(t *testing.T) {
	db := postgresFixture(t)
	cfg := productionConfig(t)
	cfg.Sources = nil
	cfg.Discovery = []Feed{{ID: "yandex", Adapter: "yandex"}, {ID: "otus", Adapter: "otus"}}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, c := range []Candidate{{Adapter: "yandex", ExternalID: "backend-developer", URL: practicumOrigin + "/backend-developer", FeedID: "yandex"}, {Adapter: "otus", ExternalID: "python-basic", URL: "https://otus.ru/lessons/python-basic", FeedID: "otus"}} {
		if _, err := db.Pool.Exec(ctx, "INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state) VALUES($1,$2,$3,$4,'published')", c.Adapter, c.ExternalID, c.URL, c.FeedID); err != nil {
			t.Fatal(err)
		}
	}
	// Practicum sorts first by older attempt but cannot consume the remaining budget.
	db.Pool.Exec(ctx, "UPDATE catalog_candidates SET attempted_at=now()-interval '1 day' WHERE adapter='otus'")
	s := Service{DB: db, Config: cfg, Worker: "pages"}
	client := clientResponse(200, string(fixtureHTML(t, "otus-python-schema.json")))
	stats, err := s.processBatch(ctx, client, s.publishDiscovered)
	if err != nil || stats.Attempted != 1 || stats.Queued != 1 {
		t.Fatal(stats, err)
	}
	var claimed bool
	db.Pool.QueryRow(ctx, "SELECT attempted_at IS NOT NULL FROM catalog_candidates WHERE adapter='yandex'").Scan(&claimed)
	if claimed {
		t.Fatal("claimed expensive item without budget")
	}
}
func TestDiscoveryStartsWithLeastRecentlyAttemptedFeed(t *testing.T) {
	db := postgresFixture(t)
	cfg := productionConfig(t)
	cfg.Discovery = []Feed{{ID: "otus", Adapter: "otus", URL: "https://otus.ru/sitemap.xml"}, {ID: "hexlet", Adapter: "hexlet", URL: "https://ru.hexlet.io/sitemap.xml"}}
	ctx := context.Background()
	db.Pool.Exec(ctx, "INSERT INTO discovery_feeds(feed_id,attempted_at) VALUES('otus',now())")
	seen := []string{}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		seen = append(seen, r.URL.Host)
		return clientResponse(200, "<urlset></urlset>").Transport.RoundTrip(r)
	})}
	s := Service{DB: db, Config: cfg}
	if _, err := s.discover(ctx, client); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "ru.hexlet.io" {
		t.Fatal(seen)
	}
}
func TestPublisherSeparatesRefreshesFromNewCoursesAndBoundsRetention(t *testing.T) {
	s := observationService(t)
	ctx := context.Background()
	event := observationFixture(t, s, time.Now().Add(-time.Minute))
	if err := s.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	r, err := s.Publish(ctx)
	if err != nil || r.NewCourses != 1 || r.RefreshedCourses != 0 {
		t.Fatal(r, err)
	}
	event.ObservedAt = event.ObservedAt.Add(time.Minute)
	event.Record.CheckedAt = event.ObservedAt
	if err = s.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	r, err = s.Publish(ctx)
	if err != nil || r.NewCourses != 0 || r.RefreshedCourses != 1 {
		t.Fatal(r, err)
	}
	// Trim at most 1,000 old done rows per pass; never delete pending observations.
	raw, _ := json.Marshal(event)
	_, err = s.DB.Pool.Exec(ctx, `INSERT INTO catalog_observations(canonical_url,observed_at,kind,payload,processed_at) SELECT $1,now()+n*interval '1 millisecond','failure',$2,now()-interval '8 days' FROM generate_series(1,1200) n`, event.Candidate.URL, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_observations WHERE processed_at<now()-interval '7 days'").Scan(&count)
	if count != 200 {
		t.Fatal("unbounded pruning", count)
	}
	_, err = s.DB.Pool.Exec(ctx, `INSERT INTO catalog_observations(canonical_url,observed_at,kind,payload,processed_at) SELECT $1,now()+n*interval '1 second','failure',$2,now() FROM generate_series(1,5001) n`, event.Candidate.URL, raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	s.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_observations WHERE processed_at IS NOT NULL").Scan(&count)
	if count != processedObservationRetention {
		t.Fatal("retention cap", count)
	}
	var b strings.Builder
	if err = Status(ctx, s.DB, &b); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"distinct_courses":1`) {
		t.Fatal(b.String())
	}
}
func TestAllProviderQueuesRotateUnderTruncatedRounds(t *testing.T) {
	db := postgresFixture(t)
	cfg := productionConfig(t)
	cfg.Sources = nil
	ctx := context.Background()
	for adapter := range providerHosts {
		cfg.Discovery = append(cfg.Discovery, Feed{ID: adapter, Adapter: adapter})
		for _, state := range []string{"pending", "published"} {
			for i := 0; i < 160; i++ {
				id := fmt.Sprintf("%s-%d", state, i)
				if _, err := db.Pool.Exec(ctx, "INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state) VALUES($1,$2,$3,$1,$4)", adapter, id, "https://"+providerHosts[adapter]+"/"+id, state); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	s := Service{DB: db, Config: cfg}
	work, err := s.queueWork(ctx)
	if err != nil || len(work) != 294 {
		t.Fatal(len(work), err)
	}
	first := work[0]
	db.Pool.Exec(ctx, "UPDATE catalog_candidates SET attempted_at=now() WHERE adapter=$1 AND external_id=$2", first.Adapter, first.ExternalID)
	again, err := s.queueWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again[0].Adapter == first.Adapter && again[0].ExternalID == first.ExternalID {
		t.Fatal("old alphabetical prefix repeatedly wins")
	}
}

func TestProviderCandidateQuotaKeepsOtherProvidersAndRefreshesAvailable(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	cfg := productionConfig(t)
	feed := Feed{ID: "otus", Adapter: "otus", URL: "https://otus.ru/sitemap.xml"}
	cfg.Discovery = []Feed{feed}
	s := Service{DB: db, Config: cfg}
	_, err := db.Pool.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id) SELECT 'otus','python-'||n,'https://otus.ru/lessons/python-'||n,'otus' FROM generate_series(1,5000) n`)
	if err != nil {
		t.Fatal(err)
	}
	client := clientResponse(200, `<urlset><url><loc>https://otus.ru/lessons/python-1</loc></url><url><loc>https://otus.ru/lessons/python-new</loc></url></urlset>`)
	seen, err := s.discoverFeed(ctx, client, feed)
	if err != nil || seen != 1 {
		t.Fatal(seen, err)
	}
	var count int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_candidates").Scan(&count)
	if count != 5000 {
		t.Fatal("provider quota exceeded", count)
	}
	feed = Feed{ID: "hexlet", Adapter: "hexlet", URL: "https://ru.hexlet.io/sitemap.xml"}
	_, err = s.discoverFeed(ctx, clientResponse(200, `<urlset><url><loc>https://ru.hexlet.io/programs/python</loc></url></urlset>`), feed)
	if err != nil {
		t.Fatal(err)
	}
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_candidates WHERE adapter='hexlet'").Scan(&count)
	if count != 1 {
		t.Fatal("large provider blocked sibling")
	}
}
func TestObservationBackpressureDoesNotDropPendingReads(t *testing.T) {
	s := observationService(t)
	ctx := context.Background()
	e := observationFixture(t, s, time.Now().Add(-time.Minute))
	raw, _ := json.Marshal(e)
	_, err := s.DB.Pool.Exec(ctx, `INSERT INTO catalog_observations(canonical_url,observed_at,kind,payload) SELECT $1,now()+n*interval '1 millisecond','discovered',$2 FROM generate_series(1,$3::int) n`, e.Candidate.URL, raw, observationQueueLimit)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.enqueue(ctx, e); err == nil {
		t.Fatal("full queue accepted more work")
	}
	var count int
	s.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_observations WHERE processed_at IS NULL").Scan(&count)
	if count != observationQueueLimit {
		t.Fatal("pending observations lost", count)
	}
}
