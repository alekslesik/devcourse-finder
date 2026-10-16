package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const aiBoundTestQuote = "Курс разработан для начинающих без опыта программирования."

func aiBoundFixtureBinding(r catalog.Course, o Observation) aiSavedBinding {
	b := aiSavedBinding{OfferID: r.Offers[0].ID, Skypro: o.SkyproProductID}
	tier := func(id int64, ts []PurpleTariff) *aiTierBinding {
		if id == 0 {
			return nil
		}
		v := &aiTierBinding{CourseID: id, Tariffs: map[string]int64{}}
		for _, t := range ts {
			v.Tariffs[t.Category] = t.ID
		}
		return v
	}
	b.Verified = tier(o.VerifiedProductID, o.VerifiedTariffs)
	b.Purple = tier(o.PurpleCourseID, o.PurpleTariffs)
	return b
}

// Synthetic section added to recorded identity/payment fixtures to exercise AI
// semantics without relying on mutable live pages or charging either provider.
func aiBoundFixtureText(raw []byte, c Candidate) []byte {
	heading := profileContracts[c.Adapter].Audience[0]
	section := `<section><h2>` + heading + `</h2><p>` + aiBoundTestQuote + ` Здесь вы научитесь разработке приложений и изучите основные конструкции языка.</p></section>`
	if !strings.Contains(string(raw), "</body>") {
		raw = []byte(string(raw) + "<body></body>")
	}
	return []byte(strings.Replace(string(raw), "</body>", section+`<aside class="related-course"><h2>`+heading+`</h2><p>FOREIGN_RECOMMENDATION</p></aside></body>`, 1))
}
func aiCheckBoundReader(t *testing.T, raw []byte, c Candidate, r catalog.Course, o Observation) {
	t.Helper()
	raw = aiBoundFixtureText(raw, c)
	b := aiBoundFixtureBinding(r, o)
	docs, e := aiBuildBoundDocuments(raw, c, r, r, o, b, fingerprint(raw))
	if e != nil {
		t.Fatal("reader", c.Adapter, e)
	}
	if len(docs) == 0 || !aiDocumentsValid(c, docs) {
		t.Fatal("invalid docs")
	}
	for _, d := range docs {
		if strings.Contains(d.Text, "FOREIGN_RECOMMENDATION") {
			t.Fatal("recommendation leaked")
		}
	}
	extraCurrent := r
	extraCurrent.Offers = append(append([]catalog.Offer(nil), r.Offers...), catalog.Offer{ID: "manual-extra", Name: "Manual additional tier", URL: c.URL})
	extraDocs, err := aiBuildBoundDocuments(raw, c, extraCurrent, r, o, b, fingerprint(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range extraDocs {
		if d.OfferID == "manual-extra" {
			t.Fatal("operator tier inferred")
		}
	}
	original := aiBundleDigest(docs)
	for _, kind := range []string{"source", "title", "language", "mapping", "product", "tariff"} {
		copyR := r
		copyB := b
		copyB.Verified = nil
		copyB.Purple = nil
		enc, _ := json.Marshal(b)
		json.Unmarshal(enc, &copyB)
		switch kind {
		case "source":
			copyR.Source += "/other"
		case "title":
			copyR.Title = "Different product"
		case "language":
			copyR.Language = "go"
		case "mapping":
			copyB.OfferID = "manual"
		case "product":
			if copyB.Verified != nil {
				copyB.Verified.CourseID++
			} else if copyB.Purple != nil {
				copyB.Purple.CourseID++
			} else if copyB.Skypro > 0 {
				copyB.Skypro++
			} else {
				continue
			}
		case "tariff":
			if copyB.Verified != nil {
				for k := range copyB.Verified.Tariffs {
					copyB.Verified.Tariffs[k]++
					break
				}
			} else if copyB.Purple != nil {
				for k := range copyB.Purple.Tariffs {
					copyB.Purple.Tariffs[k]++
					break
				}
			} else {
				continue
			}
		}
		if _, e = aiBuildBoundDocuments(raw, c, copyR, r, o, copyB, fingerprint(raw)); e == nil {
			t.Fatal("identity accepted", kind)
		}
	}
	docs[0].BindingDigest = ""
	if aiDocumentsValid(c, docs) {
		t.Fatal("missing binding accepted")
	}
	if aiBundleDigest(docs) == original {
		t.Fatal("cache ignores binding")
	}
}

type boundAIModel struct{ calls int }

func (m *boundAIModel) Extract(ctx context.Context, c Candidate, docs []AIDocument) (AIExtraction, AIUsage, error) {
	m.calls++
	return AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{{Field: "audience", DocumentID: "course", Quote: aiBoundTestQuote, Values: []string{"none"}}}}, AIUsage{Prompt: 100, Total: 100}, nil
}
func (m *boundAIModel) Verify(ctx context.Context, c Candidate, docs []AIDocument, x AIExtraction) (AIVerification, AIUsage, error) {
	m.calls++
	return AIVerification{Complete: true, Verified: []int{0}}, AIUsage{Prompt: 50, Total: 50}, nil
}
func aiCheckBoundPostgres(t *testing.T, raw []byte, c Candidate, r catalog.Course, o Observation) {
	t.Helper()
	db := postgresFixture(t)
	ctx := context.Background()
	b := aiBoundFixtureBinding(r, o)
	raw = aiBoundFixtureText(raw, c)
	// Snapshot fixtures can contain an expired sale. AI publication never refreshes
	// financial observations; a publisher timestamp now does not renew that sale.
	remote := r
	r.Profile = nil
	r.Audience = []string{"unknown"}
	r.Goals = []string{"unknown"}
	r.Direction = "unknown"
	for i := range r.Offers {
		r.Offers[i].LearningConditions = nil
	}
	if e := db.Import(ctx, catalog.Dataset{Domains: []string{providerHosts[c.Adapter]}, Courses: []catalog.Course{r}}, "operator", false); e != nil {
		t.Fatal(e)
	}
	loaded, e := db.Load(ctx)
	if e != nil {
		t.Fatal(e)
	}
	r = loaded[0]
	var verified, purple any
	if b.Verified != nil {
		verified, _ = json.Marshal(b.Verified)
	}
	if b.Purple != nil {
		purple, _ = json.Marshal(b.Purple)
	}
	_, e = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id,verified_binding,purple_binding,skypro_product_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, c.Adapter, c.ExternalID, c.URL, r.ID, b.OfferID, verified, purple, b.Skypro)
	if e != nil {
		t.Fatal(e)
	}
	docs, e := aiBuildBoundDocuments(raw, c, r, remote, o, b, fingerprint(raw))
	if e != nil {
		t.Fatal(e)
	}
	model := &boundAIModel{}
	engine := AIEngine{DB: db, Model: model, ModelName: "fixture", DailyLimit: 2}
	now := time.Now().UTC().Truncate(time.Microsecond)
	resolved, e := engine.Resolve(ctx, r.ID, c, docs, now)
	if e != nil {
		t.Fatal(e)
	}
	cached, e := engine.Resolve(ctx, r.ID, c, docs, now)
	if e != nil || !cached.Cached || model.calls != 2 {
		t.Fatal("cache", e, cached, model.calls)
	}
	event := AIEvent{Version: 1, CourseID: r.ID, IdentityDigest: aiIdentityDigest(r), Candidate: c, ObservedAt: now, Model: "fixture", Policy: aiEffectivePolicy(), Documents: docs, Extraction: resolved.Extraction, Verification: resolved.Verification}
	s := Service{DB: db, Config: productionConfig(t), Worker: "publisher"}
	if e = s.enqueueAI(ctx, event); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Pool.Exec(ctx, `UPDATE catalog_identities SET offer_id=offer_id||'-changed' WHERE course_id=$1`, r.ID); e == nil {
		t.Fatal("FK expected")
	}
	// Change the native binding while leaving public catalog identity unchanged.
	if _, e = db.Pool.Exec(ctx, `UPDATE catalog_identities SET skypro_product_id=skypro_product_id+1 WHERE course_id=$1`, r.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Publish(ctx); e != nil {
		t.Fatal(e)
	}
	loaded, e = db.Load(ctx)
	if e != nil || loaded[0].Profile != nil {
		t.Fatal("remap published", e)
	}
	if _, e = db.Pool.Exec(ctx, `UPDATE catalog_identities SET skypro_product_id=$2 WHERE course_id=$1`, r.ID, b.Skypro); e != nil {
		t.Fatal(e)
	}
	event.ObservedAt = now.Add(time.Millisecond)
	if e = s.enqueueAI(ctx, event); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Publish(ctx); e != nil {
		t.Fatal(e)
	}
	loaded, e = db.Load(ctx)
	if e != nil || loaded[0].Profile == nil || loaded[0].Profile.Audience == nil {
		t.Fatal("profile missing", e)
	}
	for i, x := range loaded[0].Offers {
		old := r.Offers[i]
		if (x.Price == nil) != (old.Price == nil) || x.Price != nil && *x.Price != *old.Price {
			t.Fatal("price changed")
		}
		if !x.PriceCheckedAt.Equal(old.PriceCheckedAt) || x.Free != old.Free || x.Enrollment != old.Enrollment || x.Schedule != old.Schedule {
			t.Fatal("financial data refreshed")
		}
		xj, _ := json.Marshal(x.Billing)
		oj, _ := json.Marshal(old.Billing)
		if string(xj) != string(oj) {
			t.Fatal("billing changed")
		}
	}
}
