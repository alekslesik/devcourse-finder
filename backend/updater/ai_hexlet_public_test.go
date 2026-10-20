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

func hexletPublicFixture(t *testing.T, slug string) ([]byte, Candidate, catalog.Course) {
	t.Helper()
	raw, err := os.ReadFile("testdata/hexlet-public-" + slug + ".html")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../data/real-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var ds catalog.Dataset
	if err = json.Unmarshal(data, &ds); err != nil {
		t.Fatal(err)
	}
	for _, r := range ds.Courses {
		if r.Source == "https://ru.hexlet.io/programs/"+slug {
			c, ok := aiCandidateURL(r.Source)
			if !ok {
				t.Fatal("candidate")
			}
			return raw, c, r
		}
	}
	t.Fatal("course missing")
	return nil, Candidate{}, catalog.Course{}
}

func TestAIHexletFreePublicIdentityAndPaidPromotionIsolation(t *testing.T) {
	for _, slug := range []string{"java-basics-free", "javascript-basics-free"} {
		t.Run(slug, func(t *testing.T) {
			raw, c, r := hexletPublicFixture(t, slug)
			// The real pages advertise a paid profession alongside the free course.
			raw = []byte(strings.Replace(string(raw), "</body>", `<section><a href="https://ru.hexlet.io/programs/java">Платная профессия: 10 месяцев, карьерная поддержка</a></section></body>`, 1))
			docs, err := aiHexletPublic(raw, c, r, nil, fingerprint(raw))
			if err != nil || !aiPartialPublicDocuments(docs) || strings.Contains(docs[0].Text, "карьерная поддержка") {
				t.Fatal("free profile or promotion isolation failed", err)
			}
			for _, bad := range []string{
				strings.ReplaceAll(string(raw), "/goto", "/other"),
				strings.ReplaceAll(string(raw), "/goto", "/goto?program=other"),
				strings.Replace(string(raw), "</body>", `<a href="https://ru.hexlet.io/programs/other/goto">Начать</a></body>`, 1),
				strings.Replace(string(raw), "</body>", `<h1>Основы другого языка</h1></body>`, 1),
				strings.Replace(string(raw), "</body>", `<script type="application/ld+json">{"@type":"Offer"}</script></body>`, 1),
			} {
				if _, err := aiHexletPublic([]byte(bad), c, r, nil, fingerprint([]byte(bad))); err == nil {
					t.Fatal("ambiguous free source accepted")
				}
			}
		})
	}
}
func TestAIHexletPublicCurrentPaidTemplatesAndForeignIdentity(t *testing.T) {
	for _, slug := range []string{"java", "frontend"} {
		t.Run(slug, func(t *testing.T) {
			raw, c, r := hexletPublicFixture(t, slug)
			docs, err := aiHexletPublic(raw, c, r, nil, fingerprint(raw))
			if err != nil || len(docs) != 1 || !docs[0].Partial || docs[0].OfferID != "" {
				t.Fatal(docs, err)
			}
			for _, bad := range []string{strings.ReplaceAll(string(raw), "/programs/"+slug, "/programs/other"), strings.ReplaceAll(string(raw), "price=premium", "price=other"), strings.Replace(string(raw), "</body>", `<script type="application/ld+json">{"@type":"Course","name":"Another"}</script></body>`, 1), strings.Replace(string(raw), "</body>", `<script type="application/ld+json">invalid</script></body>`, 1), strings.Replace(string(raw), "</body>", `<script type="application/ld+json">{"@type":["Thing","https://schema.org/Course"]}</script></body>`, 1)} {
				if _, err := aiHexletPublic([]byte(bad), c, r, nil, fingerprint([]byte(bad))); err == nil {
					t.Fatal("ambiguous source accepted")
				}
			}
			r.Title = "Different program"
			if _, err := aiHexletPublic(raw, c, r, nil, fingerprint(raw)); err == nil {
				t.Fatal("wrong course")
			}
		})
	}
}
func TestAIHexletPublicQueueIdentityCacheAndOffers(t *testing.T) {
	for _, slug := range []string{"java", "java-basics-free", "javascript-basics-free"} {
		t.Run(slug, func(t *testing.T) { checkHexletPublicQueue(t, slug) })
	}
}
func checkHexletPublicQueue(t *testing.T, slug string) {
	db := postgresFixture(t)
	raw, c, r := hexletPublicFixture(t, slug)
	raw = []byte(strings.Replace(string(raw), `<meta name="description" content="`, `<meta name="description" content="Программа предназначена для начинающих без опыта программирования. `, 1))
	r.Profile = nil
	r.Audience = []string{"unknown"}
	r.Goals = []string{"unknown"}
	r.Direction = "unknown"
	ctx := context.Background()
	if err := db.Import(ctx, catalog.Dataset{Domains: []string{"ru.hexlet.io"}, Courses: []catalog.Course{r}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	before, err := db.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r = before[0]
	_, docs, err := aiFetchBoundDocuments(ctx, db, nil, r, c, raw, fingerprint(raw))
	if err != nil {
		t.Fatal(err)
	}
	// The synthetic quote tests plumbing; no real model accuracy is implied.
	if !strings.Contains(docs[0].Text, "Программа предназначена для начинающих без опыта программирования.") {
		t.Fatal("synthetic source quote missing")
	}
	model := &publicNetologyModel{}
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
	// Adding a native mapping after collection invalidates the unbound event.
	if _, err = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id) VALUES($1,$2,$3,$4,$5)`, c.Adapter, c.ExternalID, c.URL, r.ID, r.Offers[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := db.Load(ctx)
	if err != nil || after[0].Profile != nil {
		t.Fatal(after, err)
	}
	if _, err = db.Pool.Exec(ctx, `DELETE FROM catalog_identities`); err != nil {
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
		t.Fatal("offer/financial data changed")
	}
}
func TestAIHexletPublicLiveSources(t *testing.T) {
	if os.Getenv("AI_HEXLET_PUBLIC_SOURCE_CHECK") != "1" {
		t.Skip("anonymous opt-in source-only check")
	}
	for _, slug := range []string{"java", "frontend", "java-basics-free", "javascript-basics-free"} {
		t.Run(slug, func(t *testing.T) {
			_, c, r := hexletPublicFixture(t, slug)
			client := NewClient()
			defer client.CloseIdleConnections()
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			raw, digest, err := fetch(ctx, client, c.URL)
			if err != nil {
				t.Fatal("source unavailable")
			}
			docs, err := aiHexletPublic(raw, c, r, nil, digest)
			if err != nil {
				t.Fatal("public metadata rejected")
			}
			t.Logf("scope=course partial=true bytes=%d; tariff conditions NOT verified", len(docs[0].Text))
		})
	}
}
