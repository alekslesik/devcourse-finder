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

func netologyCurrent(t *testing.T, data []byte, now time.Time) []byte {
	t.Helper()
	months := []string{"", "января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}
	start := now.Add(7 * 24 * time.Hour)
	return netologyMutate(t, data, func(d map[string]any) {
		p := netologyProgramMap(d)
		p["discount_finish_date"] = now.Add(2 * 24 * time.Hour).Format("2006-01-02")
		p["date"] = start.Format("2006-01-02")
		p["starts_at"] = start.Format("2") + " " + months[start.Month()] + " " + start.Format("2006")
	})
}
func TestNetologyPagesCollectorQueuesAndPublisherPreservesFamily(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: "netology-catalog", Adapter: "netology", URL: "https://netology.ru/development", Kind: "catalog"}}
	data, c := netologyFixture(t, "java-developer")
	data = netologyCurrent(t, data, time.Now().UTC())
	paths := map[string]int{}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		paths[r.URL.String()]++
		var b []byte
		switch r.URL.String() {
		case config.Discovery[0].URL:
			b = []byte(`<a href="/programs/java-developer">Java</a><a href="/free-lessons/java">Intro</a>`)
		case c.URL:
			b = data
		default:
			t.Fatal("unexpected endpoint", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}, Request: r}, nil
	})}
	collector := Service{DB: db, Config: config, Worker: "pages", Client: client}
	ctx := context.Background()
	r, err := collector.Run(ctx)
	if err != nil || r.Queued != 1 || r.Published != 0 || r.Failed != 0 || len(paths) != 2 {
		t.Fatal(r, paths, err)
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
	if err = json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	publisher := Service{DB: db, Config: config, Worker: "publisher"}
	r, err = publisher.Run(ctx)
	if err != nil || r.Published != 1 {
		t.Fatal(r, err)
	}
	// A next cohort may change program ID within the same family; it must not
	// permanently disable price refresh as a frozen cohort identity would.
	event.ObservedAt = time.Now().UTC()
	event.Observation.NetologyProgramID++
	if err = collector.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	r, err = publisher.Run(ctx)
	if err != nil || r.Published != 1 {
		t.Fatal("next cohort rejected", r, err)
	}
	var program, family int64
	if err = db.Pool.QueryRow(ctx, "SELECT netology_program_id,netology_family_id FROM catalog_identities").Scan(&program, &family); err != nil || program != event.Observation.NetologyProgramID || family != event.Observation.NetologyFamilyID {
		t.Fatal(program, family, err)
	}
	// Seed a real pending price anomaly, then verify a family change interrupts it.
	higher := *event.Observation.Price * 2
	event.Observation.Price = &higher
	event.ObservedAt = time.Now().UTC()
	if err = collector.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	r, err = publisher.Run(ctx)
	if err != nil || r.Published != 0 {
		t.Fatal(r, err)
	}
	event.ObservedAt = time.Now().UTC()
	event.Observation.NetologyFamilyID++
	if err = collector.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	r, err = publisher.Run(ctx)
	if err != nil || r.Published != 0 {
		t.Fatal("different family published", r, err)
	}
	var code string
	var pending int
	if err = db.Pool.QueryRow(ctx, "SELECT code FROM catalog_observations ORDER BY id DESC LIMIT 1").Scan(&code); err != nil || code != "protected_identity" {
		t.Fatal(code, err)
	}
	if err = db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&pending); err != nil || pending != 0 {
		t.Fatal("interrupted confirmation retained", pending, err)
	}
	old, _ = db.Load(ctx)
	if len(old) != 1 || *old[0].Offers[0].Price != 13170000 {
		t.Fatal(old)
	}
}
