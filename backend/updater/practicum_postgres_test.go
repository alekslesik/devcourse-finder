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

func TestPracticumCollectorQueuesAllEvidenceAndPublisherProtectsProducts(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: "yandex-sitemap", Adapter: "yandex", URL: practicumOrigin + "/lang-static/sitemap/"}}
	page, f, c := practicumData(t, "backend-developer")
	// Cohorts remain current during integration runs without making the recorded
	// API snapshot's actual dates a permanent time dependency.
	var cohorts []map[string]any
	json.Unmarshal(f.Squads, &cohorts)
	for _, cohort := range cohorts {
		cohort["payment_deadline"] = time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	}
	f.Squads, _ = json.Marshal(cohorts)
	paths := map[string]int{}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		paths[r.URL.String()]++
		var body []byte
		switch r.URL.String() {
		case config.Discovery[0].URL:
			body = []byte(`<urlset><url><loc>` + c.URL + `/</loc></url></urlset>`)
		case candidateEndpoint(c):
			body = page
		case practicumProfessionURL(c.ExternalID):
			body = f.Profession
		case practicumPricesURL(c.ExternalID):
			body = f.Prices
		case practicumSquadsURL(c.ExternalID):
			body = f.Squads
		default:
			t.Fatalf("unexpected URL %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body))), Header: http.Header{}, Request: r}, nil
	})}
	s := Service{DB: db, Config: config, Worker: "api", Client: client}
	ctx := context.Background()
	result, err := s.Run(ctx)
	if err != nil || result.Queued != 1 || result.Published != 0 || result.Failed != 0 {
		t.Fatal(result, err)
	}
	if len(paths) != 5 {
		t.Fatal("missing independent evidence", paths)
	}
	old, err := db.Load(ctx)
	if err != nil || len(old) != 0 {
		t.Fatal("collector published", old, err)
	}
	var raw []byte
	if err = db.Pool.QueryRow(ctx, "SELECT payload FROM catalog_observations").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event QueuedObservation
	if json.Unmarshal(raw, &event) != nil {
		t.Fatal("invalid event")
	}
	publisher := Service{DB: db, Config: config, Worker: "publisher"}
	result, err = publisher.Run(ctx)
	if err != nil || result.Published != 1 {
		t.Fatal(result, err)
	}
	var product, profession string
	if err = db.Pool.QueryRow(ctx, "SELECT product_id,profession_id FROM catalog_identities").Scan(&product, &profession); err != nil || product != event.Observation.ProductID || profession != event.Observation.ProfessionID {
		t.Fatal(product, profession, err)
	}
	old, _ = db.Load(ctx)
	if len(old) != 1 || old[0].Offers[0].Price == nil || *old[0].Offers[0].Price != 13700000 {
		t.Fatal(old)
	}
	event.ObservedAt = time.Now().UTC()
	event.Observation.ProductID = "11111111-1111-4111-8111-111111111111"
	if err = s.enqueue(ctx, event); err != nil {
		t.Fatal(err)
	}
	result, err = publisher.Run(ctx)
	if err != nil || result.Published != 0 {
		t.Fatal("changed product published", result, err)
	}
	var code string
	db.Pool.QueryRow(ctx, "SELECT code FROM catalog_observations ORDER BY id DESC LIMIT 1").Scan(&code)
	if code != "protected_identity" {
		t.Fatal(code)
	}
	if err = db.Pool.QueryRow(ctx, "SELECT product_id FROM catalog_identities").Scan(&product); err != nil || product == event.Observation.ProductID {
		t.Fatal("binding overwritten", product, err)
	}
}
func TestPracticumBadPageCannotCauseAdditionalFetches(t *testing.T) {
	c := Candidate{Adapter: "yandex", ExternalID: "backend-developer", URL: practicumOrigin + "/backend-developer"}
	client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("API fetched before page validation")
		return nil, ErrSource
	})}
	if _, _, _, err := fetchPracticum(context.Background(), client, []byte("<html>challenge</html>"), "", c); err == nil {
		t.Fatal("challenge accepted")
	}
}

func TestPracticumIdentityRejectionInterruptsAnomalyConfirmation(t *testing.T) {
	for _, identity := range []string{"product", "profession"} {
		for _, anomaly := range []string{"price", "closure"} {
			t.Run(identity+"/"+anomaly, func(t *testing.T) {
				db := postgresFixture(t)
				config := productionConfig(t)
				config.Sources = nil
				config.Discovery = []Feed{{ID: "yandex-sitemap", Adapter: "yandex", URL: practicumOrigin + "/lang-static/sitemap/"}}
				page, f, c := practicumData(t, "backend-developer")
				now := time.Now().UTC().Truncate(time.Microsecond)
				var cohorts []map[string]any
				if err := json.Unmarshal(f.Squads, &cohorts); err != nil {
					t.Fatal(err)
				}
				for _, cohort := range cohorts {
					cohort["payment_deadline"] = now.Add(7 * 24 * time.Hour).Format(time.RFC3339)
				}
				f.Squads, _ = json.Marshal(cohorts)
				record, o, err := collectPracticum(page, f.Profession, f.Prices, f.Squads, c, now.Add(-20*time.Hour))
				if err != nil {
					t.Fatal(err)
				}
				s := Service{DB: db, Config: config, Worker: "publisher"}
				ctx := context.Background()
				publish := func(obs Observation, at time.Time, expectedCode string, expectedCount int) {
					t.Helper()
					if err := s.enqueue(ctx, QueuedObservation{Kind: "discovered", Candidate: c, Record: record, Observation: obs, ObservedAt: at, Digest: fingerprint(f.Prices)}); err != nil {
						t.Fatal(err)
					}
					result, err := s.Publish(ctx)
					if err != nil || result.Published != expectedCount {
						t.Fatal(result, err)
					}
					var code string
					if err = db.Pool.QueryRow(ctx, "SELECT code FROM catalog_observations ORDER BY id DESC LIMIT 1").Scan(&code); err != nil || code != expectedCode {
						t.Fatal(code, err)
					}
				}
				if identity == "product" && anomaly == "price" {
					// Exercise a source whose persisted identity uses operator IDs,
					// rather than the collector's generated record ID.
					curated := record
					curated.ID = "operator-python"
					curated.Slug = curated.ID
					curated.Offers = append([]catalog.Offer(nil), record.Offers...)
					curated.Offers[0].ID = "operator-python-full"
					if err = db.Import(ctx, catalog.Dataset{Domains: []string{"practicum.yandex.ru"}, Courses: []catalog.Course{curated}}, "operator", false); err != nil {
						t.Fatal(err)
					}
				}
				publish(o, now.Add(-20*time.Hour), "verified", 1)
				changed := o
				if anomaly == "price" {
					price := *o.Price * 2
					changed.Price = &price
				} else {
					changed.Enrollment = "closed"
				}
				publish(changed, now.Add(-18*time.Hour), "pending_confirmation", 0)
				mismatched := changed
				if identity == "product" {
					mismatched.ProductID = "11111111-1111-4111-8111-111111111111"
				} else {
					mismatched.ProfessionID = "11111111-1111-4111-8111-111111111111"
				}
				publish(mismatched, now.Add(-12*time.Hour), "protected_identity", 0)
				var confirmations int
				if err = db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&confirmations); err != nil || confirmations != 0 {
					t.Fatal("rejected identity retained confirmation", confirmations, err)
				}
				// Eight hours after the first anomaly would previously confirm it. The
				// intervening identity rejection must require a new consecutive pair.
				publish(changed, now.Add(-10*time.Hour), "pending_confirmation", 0)
				old, err := db.Load(ctx)
				if err != nil || len(old) != 1 || *old[0].Offers[0].Price != *o.Price || old[0].Offers[0].Enrollment != o.Enrollment {
					t.Fatal("interrupted pair published", old, err)
				}
				publish(changed, now.Add(-3*time.Hour), "verified", 1)
				old, err = db.Load(ctx)
				if err != nil || len(old) != 1 || *old[0].Offers[0].Price != *changed.Price || old[0].Offers[0].Enrollment != changed.Enrollment {
					t.Fatal("fresh pair did not publish", old, err)
				}
			})
		}
	}
}
