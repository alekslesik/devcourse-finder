package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func sfPublicFixture(t *testing.T) ([]byte, []byte, Candidate, catalog.Course, aiSavedBinding) {
	raw, pricing, c := sfFixture(t)
	r, o, err := collectSkillfactory(raw, pricing, c, time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return raw, pricing, c, r, aiBoundFixtureBinding(r, o)
}
func TestAISkillfactoryPublicIdentityIndependentOfCampaign(t *testing.T) {
	raw, pricing, c, r, b := sfPublicFixture(t)
	// The campaign can be expired without turning metadata into a fresh price.
	pricing = []byte(strings.ReplaceAll(string(pricing), "2026-10-13", "2020-10-13"))
	docs, err := aiSkillfactoryPublic(raw, pricing, c, r, b, fingerprint(raw))
	if err != nil || len(docs) != 1 || !docs[0].Partial || docs[0].Scope != "course" || docs[0].OfferID != "" {
		t.Fatal(docs, err)
	}
	for _, mutation := range []struct {
		from, to string
		api      bool
	}{{"PDEV", "OTHER", true}, {"python-developer", "java-developer", true}, {"PDEV-BASIC", "OTHER-BASIC", false}, {"price-automation.js", "other.js", false}} {
		html, p := raw, pricing
		if mutation.api {
			p = []byte(strings.ReplaceAll(string(p), mutation.from, mutation.to))
		} else {
			html = []byte(strings.ReplaceAll(string(html), mutation.from, mutation.to))
		}
		if _, err := aiSkillfactoryPublic(html, p, c, r, b, fingerprint(html)); err == nil {
			t.Fatal("wrong identity accepted", mutation)
		}
	}
	b.Verified.CourseID++
	if _, err := aiSkillfactoryPublic(raw, pricing, c, r, b, fingerprint(raw)); err == nil {
		t.Fatal("remap accepted")
	}
}
func TestAISkillfactoryPublicPublicationAndRemap(t *testing.T) {
	db := postgresFixture(t)
	raw, pricing, c, r, b := sfPublicFixture(t)
	raw = []byte(strings.Replace(string(raw), `<meta name="description" content="`, `<meta name="description" content="Программа предназначена для начинающих без опыта программирования. `, 1))
	// The fake model uses a synthetic quote in the selected audience section.
	r.Profile = nil
	r.Audience = []string{"unknown"}
	r.Goals = []string{"unknown"}
	r.Direction = "unknown"
	ctx := context.Background()
	if err := db.Import(ctx, catalog.Dataset{Domains: []string{"skillfactory.ru"}, Courses: []catalog.Course{r}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	before, err := db.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r = before[0]
	encoded, _ := json.Marshal(b.Verified)
	if _, err = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id,verified_binding) VALUES($1,$2,$3,$4,$5,$6)`, c.Adapter, c.ExternalID, c.URL, r.ID, b.OfferID, encoded); err != nil {
		t.Fatal(err)
	}
	docs, err := aiSkillfactoryPublic(raw, pricing, c, r, b, fingerprint(raw))
	if err != nil {
		t.Fatal(err)
	}
	model := &publicNetologyModel{}
	// Use the exact synthetic quote expected by the shared mocked model.
	if !strings.Contains(docs[0].Text, "Программа предназначена для начинающих без опыта программирования.") {
		t.Fatal("synthetic source quote missing")
	}
	engine := AIEngine{DB: db, Model: model, ModelName: "test", DailyLimit: 2}
	at := time.Now().UTC().Truncate(time.Microsecond)
	resolved, err := engine.Resolve(ctx, r.ID, c, docs, at)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := engine.Resolve(ctx, r.ID, c, docs, at)
	if err != nil || !cached.Cached || model.calls != 2 {
		t.Fatal(cached, err)
	}
	event := AIEvent{Version: 1, CourseID: r.ID, IdentityDigest: aiIdentityDigest(r), Candidate: c, ObservedAt: at, Model: "test", Policy: aiEffectivePolicy(), Documents: docs, Extraction: resolved.Extraction, Verification: resolved.Verification}
	publisher := Service{DB: db, Config: productionConfig(t), Worker: "publisher"}
	if err = publisher.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE catalog_identities SET skypro_product_id=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := db.Load(ctx)
	if err != nil || after[0].Profile != nil {
		t.Fatal(after, err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE catalog_identities SET skypro_product_id=0`); err != nil {
		t.Fatal(err)
	}
	event.ObservedAt = at.Add(time.Millisecond)
	if err = publisher.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	after, err = db.Load(ctx)
	if err != nil || after[0].Profile == nil {
		t.Fatal(after, err)
	}
	if !reflect.DeepEqual(before[0].Offers, after[0].Offers) || !before[0].CheckedAt.Equal(after[0].CheckedAt) {
		t.Fatal("offer/financial freshness changed")
	}
}
func TestAISkillfactoryPublicLiveSource(t *testing.T) {
	if os.Getenv("AI_SKILLFACTORY_PUBLIC_SOURCE_CHECK") != "1" {
		t.Skip("explicit anonymous source check only")
	}
	_, _, c, r, b := sfPublicFixture(t)
	client := NewClient()
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	raw, digest, err := fetch(ctx, client, c.URL)
	if err != nil {
		t.Fatal("source unavailable")
	}
	docs, err := aiFetchSkillfactoryPublic(ctx, client, raw, c, r, b, digest)
	if err != nil {
		t.Fatal("public source contract rejected", rejectionCode(err, "scope_rejected"))
	}
	t.Logf("scope=course partial=true bytes=%d; prices and tariff conditions NOT verified", len(docs[0].Text))
}
