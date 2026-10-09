package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPurpleSchoolQueueMultipleTariffsAndConfirmation(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: "purpleschool-sitemap", Adapter: "purpleschool", URL: "https://purpleschool.ru/sitemap.xml"}}
	b, c := purpleFixture(t, "go-basics")
	requests := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		requests++
		body := string(b)
		if r.URL.String() == config.Discovery[0].URL {
			body = `<urlset><url><loc>https://purpleschool.ru/course/go-basics</loc></url></urlset>`
		} else if r.URL.String() != c.URL {
			t.Fatal(r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	collector := Service{DB: db, Config: config, Worker: "pages", Client: client}
	ctx := context.Background()
	result, err := collector.Run(ctx)
	if err != nil || result.Queued != 1 || result.Published != 0 || requests != 2 {
		t.Fatal(result, requests, err)
	}
	old, _ := db.Load(ctx)
	if len(old) != 0 {
		t.Fatal("collector published")
	}
	var raw []byte
	if err = db.Pool.QueryRow(ctx, "SELECT payload FROM catalog_observations").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event QueuedObservation
	json.Unmarshal(raw, &event)
	publisher := Service{DB: db, Config: config, Worker: "publisher", Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) { t.Fatal("publisher fetched", r.URL); return nil, nil })}}
	result, err = publisher.Run(ctx)
	if err != nil || result.Published != 1 {
		t.Fatal(result, err)
	}
	old, _ = db.Load(ctx)
	if len(old) != 1 || len(old[0].Offers) != 3 || old[0].Offers[0].Free || old[0].Offers[1].Mentor || !old[0].Offers[2].Mentor {
		t.Fatal(old)
	}
	// A large price change needs another window; ordinary tariffs still refresh.
	oldPrice := event.Observation.PurpleTariffs[0].Price
	event.Observation.PurpleTariffs[0].Price *= 2
	event.Observation.Price = &event.Observation.PurpleTariffs[0].Price
	event.Record.Offers[0].Price = event.Observation.Price
	event.ObservedAt = time.Now().UTC()
	if err = collector.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	result, err = publisher.Run(ctx)
	if err != nil {
		t.Fatal(result, err)
	}
	old, _ = db.Load(ctx)
	basePrice := func() int64 {
		for _, offer := range old[0].Offers {
			if offer.ID == event.Record.Offers[0].ID {
				return *offer.Price
			}
		}
		t.Fatal("missing base tariff")
		return 0
	}
	if basePrice() != oldPrice {
		t.Fatal("unconfirmed price published")
	}
	// Move only the disposable confirmation timestamp, simulating a later window.
	_, err = db.Pool.Exec(ctx, "UPDATE updater_candidates SET observed_at=now()-interval '7 hours'")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().UTC().Add(26 * time.Hour)
	event.Observation.ValidUntil = &deadline
	for i := range event.Record.Offers {
		event.Record.Offers[i].ValidUntil = &deadline
	}
	event.ObservedAt = time.Now().UTC()
	if err = collector.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	result, err = publisher.Run(ctx)
	if err != nil {
		t.Fatal(result, err)
	}
	old, _ = db.Load(ctx)
	if basePrice() != 2*oldPrice {
		t.Fatal("rolling verification lease prevented confirmation", old)
	}
	// A failed read must clear confirmations for every tariff of this course.
	_, err = db.Pool.Exec(ctx, "INSERT INTO updater_candidates VALUES($1,'pending',now(),1)", event.Record.ID+":50")
	if err != nil {
		t.Fatal(err)
	}
	failure := QueuedObservation{Kind: "failure", Candidate: c, ObservedAt: time.Now().UTC(), FailureCode: "invalid_purpleschool_contract"}
	if err = collector.enqueue(ctx, failure); err != nil {
		t.Fatal(err)
	}
	_, err = publisher.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var count int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&count)
	if count != 0 {
		t.Fatal("failure retained tariff confirmation", count)
	}
	// Rebinding a page to another provider course cannot replace the old course.
	event.Observation.PurpleCourseID++
	event.ObservedAt = time.Now().UTC()
	if err = collector.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	result, err = publisher.Run(ctx)
	if err != nil || result.Published != 0 {
		t.Fatal(result, err)
	}
	var code string
	db.Pool.QueryRow(ctx, "SELECT code FROM catalog_observations ORDER BY id DESC LIMIT 1").Scan(&code)
	if code != "protected_identity" {
		t.Fatal(code)
	}
	event.Observation.PurpleCourseID--
	event.Observation.PurpleTariffs[0].ID += 1000
	event.Record.Offers[0].ID = event.Record.ID + "-t" + strconv.FormatInt(event.Observation.PurpleTariffs[0].ID, 10)
	event.ObservedAt = time.Now().UTC()
	if err = collector.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	result, err = publisher.Run(ctx)
	if err != nil || result.Published != 0 {
		t.Fatal("changed tariff identity published", result, err)
	}
	db.Pool.QueryRow(ctx, "SELECT code FROM catalog_observations ORDER BY id DESC LIMIT 1").Scan(&code)
	if code != "protected_identity" {
		t.Fatal(code)
	}
}
func TestPurpleSchoolRejectsTamperedQueueTariffs(t *testing.T) {
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: "purpleschool-sitemap", Adapter: "purpleschool", URL: "https://purpleschool.ru/sitemap.xml"}}
	b, c := purpleFixture(t, "go-basics")
	r, o, err := collectPurpleSchool(b, c, netologyNow())
	if err != nil {
		t.Fatal(err)
	}
	event := QueuedObservation{Version: 1, Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: netologyNow(), Digest: fingerprint(b)}
	if err = event.validate(config); err != nil {
		t.Fatal(err)
	}
	event.Record.Offers[1].Mentor = true
	if event.validate(config) == nil {
		t.Fatal("AI tariff became human mentorship")
	}
	event.Record.Offers[1].Mentor = false
	event.Observation.PurpleTariffs[1].ID = event.Observation.PurpleTariffs[0].ID
	if event.validate(config) == nil {
		t.Fatal("duplicate tariff identity accepted")
	}
}

func TestPurpleSchoolPublishesStandaloneFreeCourse(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: "purpleschool-sitemap", Adapter: "purpleschool", URL: "https://purpleschool.ru/sitemap.xml"}}
	b, c := purpleFixture(t, "python-start")
	now := time.Now().UTC()
	r, o, err := collectPurpleSchool(b, c, now)
	if err != nil {
		t.Fatal(err)
	}
	collector := Service{DB: db, Config: config, Worker: "pages"}
	event := QueuedObservation{Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: now, Digest: fingerprint(b)}
	if err = collector.enqueue(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	publisher := Service{DB: db, Config: config, Worker: "publisher"}
	result, err := publisher.Run(context.Background())
	if err != nil || result.Published != 1 {
		t.Fatal(result, err)
	}
	old, err := db.Load(context.Background())
	if err != nil || len(old) != 1 || len(old[0].Offers) != 1 || !old[0].Offers[0].Free || *old[0].Offers[0].Price != 0 {
		t.Fatal(old, err)
	}
}

func TestPurpleSchoolLegacyFailureInterruptsTariffConfirmations(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusOK} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			db := postgresFixture(t)
			ctx := context.Background()
			config := productionConfig(t)
			config.Sources = nil
			config.Discovery = []Feed{{ID: "purpleschool-sitemap", Adapter: "purpleschool", URL: "https://purpleschool.ru/sitemap.xml"}}
			b, c := purpleFixture(t, "go-basics")
			record, o, err := collectPurpleSchool(b, c, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			service := Service{DB: db, Config: config} // Empty Worker exercises legacy publication.
			code, n, err := service.publishDiscovered(ctx, c, record, o, fingerprint(b))
			if err != nil || code != "verified" || n != 1 {
				t.Fatal(code, n, err)
			}
			for i := 0; i < 2; i++ {
				o.PurpleTariffs[i].Price *= 2
				price := o.PurpleTariffs[i].Price
				record.Offers[i].Price = &price
			}
			o.Price = &o.PurpleTariffs[0].Price
			code, _, err = service.publishDiscovered(ctx, c, record, o, fingerprint(b))
			if err != nil || code != "pending_confirmation" {
				t.Fatal(code, err)
			}
			// Independent confirmations exist for both anomalous tariffs.
			var count int
			db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&count)
			if count != 2 {
				t.Fatal(count)
			}
			_, err = db.Pool.Exec(ctx, "UPDATE updater_candidates SET observed_at=now()-interval '7 hours'")
			if err != nil {
				t.Fatal(err)
			}
			unrelated := record.ID + "-other:50"
			_, err = db.Pool.Exec(ctx, "INSERT INTO updater_candidates VALUES($1,'unrelated',now(),1)", unrelated)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Pool.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state) VALUES($1,$2,$3,$4,'published')`, c.Adapter, c.ExternalID, c.URL, c.FeedID)
			if err != nil {
				t.Fatal(err)
			}
			stats, err := service.processBatch(ctx, clientResponse(status, "<html>challenge</html>"), service.publishDiscovered)
			if err != nil || stats.Attempted != 1 || stats.Failed != 1 {
				t.Fatal(stats, err)
			}
			db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates WHERE source_id<>$1", unrelated).Scan(&count)
			if count != 0 {
				t.Fatal("legacy failure retained tariff confirmations", count)
			}
			db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates WHERE source_id=$1", unrelated).Scan(&count)
			if count != 1 {
				t.Fatal("unrelated confirmation removed")
			}
			// A later matching read must start fresh despite the elapsed confirmation window.
			code, _, err = service.publishDiscovered(ctx, c, record, o, fingerprint(b))
			if err != nil || code != "pending_confirmation" {
				t.Fatal("price accepted across failed fetch", code, err)
			}
			courses, _ := db.Load(ctx)
			for _, offer := range courses[0].Offers {
				if offer.ID == record.Offers[0].ID && *offer.Price != 399900 {
					t.Fatal("unconfirmed price published", offer)
				}
			}
		})
	}
}
