package updater

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCollectorsOnlyQueueOwnedReadsAndPublisherDoesNotFetch(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	config.Discovery = []Feed{{ID: "stepik", Adapter: "stepik", URL: "https://stepik.org/sitemap.xml"}, {ID: "otus", Adapter: "otus", URL: "https://otus.ru/sitemap.xml"}}
	ctx := context.Background()
	for _, role := range []string{"api", "pages"} {
		client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			if owner(map[string]string{"stepik.org": "stepik", "otus.ru": "otus"}[r.URL.Host]) != role {
				t.Fatal("wrong role host", role, r.URL)
			}
			var body []byte
			var err error
			if strings.HasSuffix(r.URL.Path, "sitemap.xml") {
				url := "https://stepik.org/course/54403/promo"
				if role == "pages" {
					url = "https://otus.ru/lessons/python-basic"
				}
				body = []byte(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>` + url + `</loc></url></urlset>`)
			} else if role == "pages" {
				body = fixtureHTML(t, "otus-python-schema.json")
			} else {
				file := "stepik-go.json"
				if strings.HasSuffix(r.URL.Path, "58852") {
					file = "stepik-python.json"
				}
				body, err = os.ReadFile("testdata/" + file)
				if err != nil {
					t.Fatal(err)
				}
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}, Request: r}, nil
		})}
		s := Service{DB: db, Config: config, Worker: role, Client: client}
		result, err := s.Run(ctx)
		if err != nil || result.Published != 0 || result.Queued == 0 {
			t.Fatal(role, result, err)
		}
		old, _ := db.Load(ctx)
		if len(old) != 0 {
			t.Fatal("collector published", role)
		}
	}
	var apiRuns, pageRuns int
	db.Pool.QueryRow(ctx, "SELECT count(*) FILTER(WHERE worker='api'),count(*) FILTER(WHERE worker='pages') FROM updater_runs").Scan(&apiRuns, &pageRuns)
	if apiRuns != 1 || pageRuns != 1 {
		t.Fatal("role schedules not independent", apiRuns, pageRuns)
	}
	publisher := Service{DB: db, Config: config, Worker: "publisher", Client: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("publisher fetched source")
		return nil, ErrSource
	})}}
	result, err := publisher.Run(ctx)
	if err != nil || result.Published != 4 {
		t.Fatal(result, err)
	}
	old, err := db.Load(ctx)
	if err != nil || len(old) != 3 {
		t.Fatal("duplicate aliases or lost courses", len(old), err)
	}
	var pending int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_observations WHERE processed_at IS NULL").Scan(&pending)
	if pending != 0 {
		t.Fatal("pending observations", pending)
	}
}
func TestWorkerLocksAndHealthAreIsolated(t *testing.T) {
	s := observationService(t)
	ctx := context.Background()
	lock, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(829105),pg_advisory_xact_lock(829106)"); err != nil {
		t.Fatal(err)
	}
	api := *s
	api.Worker = "api"
	if _, err = api.Run(ctx); !errors.Is(err, ErrBusy) {
		t.Fatal("same role overlap", err)
	}
	if _, err = s.Run(ctx); !errors.Is(err, ErrBusy) {
		t.Fatal("legacy mixed with split worker", err)
	}
	pages := *s
	pages.Worker = "pages"
	pages.Config.Discovery = []Feed{{ID: "otus", Adapter: "otus", URL: "https://otus.ru/sitemap.xml"}}
	pages.Client = clientResponse(200, `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"></urlset>`)
	if _, err = pages.Run(ctx); err != nil {
		t.Fatal("API lock blocked page collector", err)
	}
	if err = heartbeatWorker(ctx, s.DB, "api"); err != nil {
		t.Fatal(err)
	}
	if HealthWorker(ctx, s.DB, "api") != nil {
		t.Fatal("API heartbeat unhealthy")
	}
	if HealthWorker(ctx, s.DB, "pages") == nil || HealthWorker(ctx, s.DB, "publisher") == nil {
		t.Fatal("another role's heartbeat accepted")
	}
}
func TestPublisherServeHasIndependentHeartbeatAndStops(t *testing.T) {
	s := observationService(t)
	s.Worker = "publisher"
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Serve(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for HealthWorker(context.Background(), s.DB, "publisher") != nil {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("publisher never became live")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publisher did not stop")
	}
}
