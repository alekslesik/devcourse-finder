package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

func sharedPracticum(t *testing.T) ([]byte, practicumFixture, practicumFixture, Candidate) {
	t.Helper()
	_, base, c := practicumData(t, "backend-developer")
	_, plus, _ := practicumData(t, "python-developer-plus")
	page, err := os.ReadFile("testdata/practicum-shared-page.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []*practicumFixture{&base, &plus} {
		var cohorts []map[string]any
		json.Unmarshal(f.Squads, &cohorts)
		for _, x := range cohorts {
			x["payment_deadline"] = time.Now().Add(24 * time.Hour).Format(time.RFC3339)
		}
		f.Squads, _ = json.Marshal(cohorts)
	}
	return page, base, plus, c
}
func sharedPracticumClient(t *testing.T, base, plus practicumFixture, failPlus bool) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		slug := r.URL.Query().Get("slugs")
		if strings.Contains(r.URL.Path, "python-developer-plus") {
			slug = "python-developer-plus"
		}
		f := base
		status := 200
		if slug == "python-developer-plus" {
			f = plus
			if failPlus {
				status = 503
			}
		}
		var b []byte
		switch {
		case strings.Contains(r.URL.Path, "professions-by-slugs"):
			b = f.Profession
		case strings.Contains(r.URL.Path, "/prices/"):
			b = f.Prices
		case strings.HasSuffix(r.URL.Path, "/nearest_squads/"):
			b = f.Squads
		default:
			t.Fatalf("unexpected URL %s", r.URL)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(string(b))), Header: http.Header{}, Request: r}, nil
	})}
}
func TestPracticumSharedLandingDoesNotUseGlobalPriceMapAsBinding(t *testing.T) {
	page, base, plus, c := sharedPracticum(t)
	for _, raw := range [][]byte{page, []byte(strings.ReplaceAll(string(page), "/profile/python-developer-plus/", "/profile/other/"))} {
		r, o, _, err := fetchPracticumGroup(context.Background(), sharedPracticumClient(t, base, plus, false), raw, fingerprint(raw), c)
		if err != nil {
			t.Fatal(err)
		}
		expected := 2
		if !strings.Contains(string(raw), "/profile/python-developer-plus/") {
			expected = 1
		}
		if len(r.Offers) != expected {
			t.Fatal(r)
		}
		if expected == 2 {
			if *r.Offers[0].Price == *r.Offers[1].Price || r.Offers[1].URL != c.URL || r.Offers[1].Name != "Расширенная программа" {
				t.Fatal("collapsed tariff", r)
			}
			e := QueuedObservation{Version: 1, Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: r.CheckedAt, Digest: fingerprint(raw)}
			if !validPracticumGroup(e) {
				t.Fatal("invalid normalized group", o)
			}
		}
	}
	// Go plus binds its explicit landing_path to the parent; no synthetic plus URL.
	_, goBase, goC := practicumData(t, "go-developer-basic")
	var p []map[string]any
	json.Unmarshal(plus.Profession, &p)
	p[0]["landing_path"] = "/wrong-course/"
	bad, _ := json.Marshal(p)
	if _, _, err := collectPracticumTariff(page, bad, plus.Prices, plus.Squads, c, "python-developer-plus", time.Now()); err == nil {
		t.Fatal("wrong parent accepted")
	}
	_, goPlus, _ := practicumData(t, "go-developer-plus")
	goPage := []byte(strings.ReplaceAll(strings.ReplaceAll(string(page), "backend-developer", "go-developer-basic"), "python-developer-plus", "go-developer-plus"))
	if _, _, err := collectPracticumTariff(goPage, goPlus.Profession, goPlus.Prices, goPlus.Squads, goC, "go-developer-plus", practicumNow()); err != nil {
		t.Fatal("shared Go landing", err)
	}
	_ = goBase
}
func TestPracticumSharedTariffPublicationIsolationIdentityAndAtomicAck(t *testing.T) {
	db := postgresFixture(t)
	page, base, plus, c := sharedPracticum(t)
	cfg := productionConfig(t)
	cfg.Sources = nil
	cfg.Discovery = []Feed{{ID: c.FeedID, Adapter: "yandex", URL: practicumOrigin + "/lang-static/sitemap/"}}
	s := Service{DB: db, Config: cfg, Worker: "publisher"}
	ctx := context.Background()
	r, o, _, err := fetchPracticumGroup(ctx, sharedPracticumClient(t, base, plus, false), page, fingerprint(page), c)
	if err != nil {
		t.Fatal(err)
	}
	publish := func(obs Observation, at time.Time) {
		t.Helper()
		if err := s.enqueue(ctx, QueuedObservation{Kind: "discovered", Candidate: c, Record: r, Observation: obs, ObservedAt: at, Digest: fingerprint(page)}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Publish(ctx); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	// Migration reuses a curated parent/offer ID while keeping the verification lease.
	curated := r
	curated.ID = "operator-python"
	curated.Slug = curated.ID
	curated.Offers = append([]catalog.Offer(nil), r.Offers[:1]...)
	curated.Offers[0].ID = "operator-python-full"
	curated.Offers[0].PriceCheckedAt = now.Add(-24 * time.Hour)
	curated.CheckedAt = now.Add(-24 * time.Hour)
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"practicum.yandex.ru"}, Courses: []catalog.Course{curated}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, "INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state,attempted_at) VALUES($1,$2,$3,$4,'pending',$5)", c.Adapter, c.ExternalID, c.URL, c.FeedID, now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	publish(o, now.Add(-20*time.Hour))
	courses, _ := db.Load(ctx)
	if len(courses) != 1 || len(courses[0].Offers) != 2 {
		t.Fatal(courses)
	}
	if courses[0].ID != "operator-python" {
		t.Fatal("curated identity lost", courses)
	}
	for _, offer := range courses[0].Offers {
		if offer.ID == "operator-python-full" && offer.ValidUntil == nil {
			t.Fatal("curated verification lease lost")
		}
	}
	// A manual third tariff is preserved across automatic refreshes.
	manual := courses[0].Offers[0]
	manual.ID = "manual-python"
	manual.Name = "Operator tariff"
	courses[0].Offers = append(courses[0].Offers, manual)
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"practicum.yandex.ru"}, Courses: courses}, "operator", false); err != nil {
		t.Fatal(err)
	}
	changed := o
	changed.PracticumTariffs = append([]PracticumTariff(nil), o.PracticumTariffs...)
	changed.PracticumTariffs[1].Price *= 2
	r.Offers = append([]catalog.Offer(nil), r.Offers...)
	price := changed.PracticumTariffs[1].Price
	r.Offers[1].Price = &price
	publish(changed, now.Add(-18*time.Hour))
	failed := o
	failed.PracticumTariffs = append([]PracticumTariff(nil), o.PracticumTariffs...)
	failed.PracticumTariffs[1] = PracticumTariff{Slug: "python-developer-plus", FailureCode: "source_unavailable"}
	saved := r
	r.Offers = r.Offers[:1]
	publish(failed, now.Add(-12*time.Hour))
	r = saved
	publish(changed, now.Add(-10*time.Hour))
	courses, _ = db.Load(ctx)
	if sharedPlusPrice(courses[0]) == price || len(courses[0].Offers) != 3 {
		t.Fatal("failed read confirmed or manual tariff lost", courses)
	}
	publish(changed, now.Add(-3*time.Hour))
	courses, _ = db.Load(ctx)
	if sharedPlusPrice(courses[0]) != price {
		t.Fatal("new pair failed", courses)
	}
	changed.PracticumTariffs[1].ProductID = "11111111-1111-4111-8111-111111111111"
	publish(changed, now.Add(-2*time.Hour))
	var code string
	db.Pool.QueryRow(ctx, "SELECT code FROM catalog_observations ORDER BY id DESC LIMIT 1").Scan(&code)
	if code != "partial_tariffs" {
		t.Fatal(code)
	}
	var next time.Time
	if err = db.Pool.QueryRow(ctx, "SELECT next_attempt_at FROM catalog_candidates WHERE adapter='yandex'").Scan(&next); err != nil || next.After(time.Now().Add(13*time.Hour)) {
		t.Fatal("one protected tariff delayed siblings", next, err)
	}
	// A rejected acknowledgement rolls back catalog changes and tariff bindings.
	_, err = db.Pool.Exec(ctx, `CREATE FUNCTION reject_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'test acknowledgement failure'; END $$; CREATE TRIGGER reject_ack BEFORE UPDATE ON catalog_observations FOR EACH ROW EXECUTE FUNCTION reject_ack()`)
	if err != nil {
		t.Fatal(err)
	}
	r.Offers[1].Price = &o.PracticumTariffs[1].Price
	if err = s.enqueue(ctx, QueuedObservation{Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: now.Add(-time.Hour), Digest: fingerprint(page)}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err == nil {
		t.Fatal("ack failure ignored")
	}
	var pending int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_observations WHERE processed_at IS NULL").Scan(&pending)
	if pending != 1 {
		t.Fatal("failed transaction acknowledged")
	}
}

func sharedPlusPrice(c catalog.Course) int64 {
	for _, o := range c.Offers {
		if strings.HasSuffix(o.ID, "python-developer-plus") {
			return *o.Price
		}
	}
	return -1
}

func TestPracticumDedicatedPlusCardIsNotItsOwnSibling(t *testing.T) {
	page, _, _, c := sharedPracticum(t)
	c.ExternalID = "python-developer-plus"
	c.URL = practicumOrigin + "/" + c.ExternalID
	page = []byte(strings.ReplaceAll(string(page), "backend-developer", "python-developer-plus"))
	slugs, err := practicumDisplayedTariffs(page, c)
	if err != nil || len(slugs) != 0 {
		t.Fatal("self tariff became a sibling", slugs, err)
	}
}
