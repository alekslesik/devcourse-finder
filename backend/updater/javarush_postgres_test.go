package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestJavaRushQueueStoreAndRecurringFeeConfirmation(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	page, pricing, c := jrFixture(t)
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: c.FeedID, Adapter: c.Adapter, URL: c.URL, Kind: "catalog"}}
	collector := Service{DB: db, Config: config, Worker: "pages"}
	publisher := Service{DB: db, Config: config, Worker: "publisher"}
	_, e := db.Pool.Exec(ctx, "INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id) VALUES($1,$2,$3,$4)", c.Adapter, c.ExternalID, c.URL, c.FeedID)
	if e != nil {
		t.Fatal(e)
	}
	client := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		b := page
		switch req.URL.String() {
		case c.URL:
		case jrMonthlyEndpoint:
			b = pricing
		default:
			t.Fatal(req.URL)
		}
		if req.Method != "GET" {
			t.Fatal("non-read request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}, Request: req}, nil
	})}
	run := func() {
		t.Helper()
		_, e := db.Pool.Exec(ctx, "UPDATE catalog_candidates SET next_attempt_at=now()")
		if e != nil {
			t.Fatal(e)
		}
		stats, e := collector.processBatch(ctx, client, collector.publishDiscovered)
		if e != nil || stats.Queued != 1 || stats.Failed != 0 {
			t.Fatal(stats, e)
		}
		if _, e = publisher.Run(ctx); e != nil {
			t.Fatal(e)
		}
	}
	run()
	courses, e := db.Load(ctx)
	if e != nil || len(courses) != 1 || courses[0].Offers[0].Billing == nil || courses[0].Offers[0].Billing.AmountMinor != 3000 || courses[0].Offers[0].Price != nil {
		t.Fatal(courses, e)
	}
	raw, _ := json.Marshal(courses[0])
	if !strings.Contains(string(raw), `"billing"`) || !strings.Contains(string(raw), `30 USD в месяц`) {
		t.Fatal("persisted presentation lost recurring fee")
	}
	// A doubled recurring fee needs another scheduled read, just like an exact
	// course-price anomaly. Rolling leases must not invalidate that confirmation.
	page = []byte(strings.ReplaceAll(string(page), ">30<", ">60<"))
	pricing = []byte(strings.ReplaceAll(string(pricing), `"usd": 30`, `"usd": 60`))
	run()
	courses, _ = db.Load(ctx)
	if courses[0].Offers[0].Billing.AmountMinor != 3000 {
		t.Fatal("unconfirmed fee published")
	}
	_, e = db.Pool.Exec(ctx, "UPDATE updater_candidates SET observed_at=now()-interval '7 hours'")
	if e != nil {
		t.Fatal(e)
	}
	run()
	courses, _ = db.Load(ctx)
	if courses[0].Offers[0].Billing.AmountMinor != 6000 || !strings.Contains(courses[0].Offers[0].Name, "60 USD") || courses[0].Offers[0].Free {
		t.Fatal("fee confirmation failed", courses)
	}
	// A foreign-currency subscription must still never become an exact RUB total.
	r, o, e := collectJavaRush(page, pricing, c, time.Now())
	if e != nil {
		t.Fatal(e)
	}
	evidence := QueuedObservation{Version: 1, Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: r.CheckedAt, Digest: fingerprint(page)}
	if evidence.validate(config) != nil {
		t.Fatal("valid billing observation rejected")
	}
	invented := int64(6000)
	evidence.Record.Offers[0].Price = &invented
	evidence.Record.Offers[0].PriceKind = "exact"
	if evidence.validate(config) == nil {
		t.Fatal("fee accepted as full RUB price")
	}
}
