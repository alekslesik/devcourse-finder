package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestAINetologySelectedPublicDurationOnly(t *testing.T) {
	_, c, r, b := aiNetologyFixture(t)
	raw, err := os.ReadFile("testdata/netology-public-go-duration.html")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := aiNetologyDocuments(raw, c, r, b, fingerprint(raw), time.Now())
	if err != nil || len(docs) != 2 {
		t.Fatal(docs, err)
	}
	d := docs[1]
	if !d.Partial || d.Scope != "offer" || d.OfferID != b.OfferID || d.Text != "10 месяцев обучения" {
		t.Fatal(d)
	}
	if !aiNetologyPublicBundle(docs, b) || docs[0].BindingDigest != aiNetologyPublicTariffDigest(r.ID, b) {
		t.Fatal("unbound bundle")
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `class="styles_header__WXLp2"`, `class="other"`, 1),
		strings.Replace(string(raw), `Go-разработчик с нуля</p>`, `Other plan</p>`, 1),
		strings.Replace(string(raw), `10 месяцев обучения`, `36 месяцев рассрочки`, 1),
		strings.Replace(string(raw), `10 месяцев обучения</p>`, `10 месяцев обучения</p><p class="styles_header__WXLp2">12 месяцев обучения</p>`, 1),
	} {
		if bad == string(raw) {
			t.Fatal("mutation missed")
		}
		docs, err := aiNetologyDocuments([]byte(bad), c, r, b, fingerprint([]byte(bad)), time.Now())
		if err == nil && len(docs) > 1 {
			t.Fatal("ambiguous offer accepted")
		}
	}
	r.Offers[0].Name = "Another plan"
	docs, err = aiNetologyDocuments(raw, c, r, b, fingerprint(raw), time.Now())
	if err != nil || len(docs) != 1 {
		t.Fatal("operator offer inferred", docs, err)
	}
}

type selectedDurationModel struct{}

func (*selectedDurationModel) Extract(_ context.Context, c Candidate, d []AIDocument) (AIExtraction, AIUsage, error) {
	return AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{{Field: "duration", DocumentID: d[1].ID, OfferID: d[1].OfferID, Quote: d[1].Text, Min: valueFloat(10), Max: valueFloat(10), Unit: "month"}}}, AIUsage{Prompt: 100, Total: 100}, nil
}
func (*selectedDurationModel) Verify(_ context.Context, _ Candidate, _ []AIDocument, _ AIExtraction) (AIVerification, AIUsage, error) {
	return AIVerification{Complete: true, Verified: []int{0}}, AIUsage{Prompt: 50, Total: 50}, nil
}
func TestAINetologySelectedDurationPublication(t *testing.T) {
	db := postgresFixture(t)
	_, c, r, b := aiNetologyFixture(t)
	ctx := context.Background()
	r.Offers[0].LearningConditions = nil
	r.Offers[0].DurationMonths = nil
	if err := db.Import(ctx, catalog.Dataset{Domains: []string{"netology.ru"}, Courses: []catalog.Course{r}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	before, err := db.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r = before[0]
	if _, err = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id,netology_family_id,netology_program_id) VALUES('netology',$1,$2,$3,$4,$5,$6)`, c.ExternalID, c.URL, r.ID, b.OfferID, b.FamilyID, b.ProgramID); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/netology-public-go-duration.html")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := aiNetologyDocuments(raw, c, r, b, fingerprint(raw), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	engine := AIEngine{DB: db, Model: &selectedDurationModel{}, ModelName: "test", DailyLimit: 2}
	at := time.Now().UTC().Truncate(time.Microsecond)
	result, err := engine.Resolve(ctx, r.ID, c, docs, at)
	if err != nil {
		t.Fatal(err)
	}
	cached, err := engine.Resolve(ctx, r.ID, c, docs, at)
	if err != nil || !cached.Cached {
		t.Fatal(cached, err)
	}
	event := AIEvent{Version: 1, CourseID: r.ID, IdentityDigest: aiIdentityDigest(r), Candidate: c, ObservedAt: at, Policy: aiEffectivePolicy(), Model: "test", Documents: docs, Extraction: result.Extraction, Verification: result.Verification}
	publisher := Service{DB: db, Config: productionConfig(t), Worker: "publisher"}
	if err = publisher.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE catalog_identities SET netology_program_id=netology_program_id+1`); err != nil {
		t.Fatal(err)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.Load(ctx)
	if err != nil || loaded[0].Offers[0].LearningConditions != nil {
		t.Fatal(loaded, err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE catalog_identities SET netology_program_id=$1`, b.ProgramID); err != nil {
		t.Fatal(err)
	}
	event.ObservedAt = at.Add(time.Millisecond)
	if err = publisher.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err = db.Load(ctx)
	if err != nil || loaded[0].Offers[0].LearningConditions == nil || loaded[0].Offers[0].LearningConditions.Duration.Max != 10 {
		t.Fatal(loaded, err)
	}
	old, now := r.Offers[0], loaded[0].Offers[0]
	if *old.Price != *now.Price || !old.PriceCheckedAt.Equal(now.PriceCheckedAt) || old.Enrollment != now.Enrollment || !r.CheckedAt.Equal(loaded[0].CheckedAt) {
		t.Fatal("financial freshness changed")
	}
}
func TestAIPartialOfferPreservesOmittedSupport(t *testing.T) {
	// A duration-only event must preserve independently verified support omitted
	// by the scoped input, rather than treating that omission as disappearance.
	_, record, e := aiEventFixture(t)
	code := ""
	first, err := mergeAI([]catalog.Course{record}, e, &code)
	if err != nil || len(first) != 1 {
		t.Fatal(err)
	}
	e.IdentityDigest = aiIdentityDigest(first[0])
	e.Extraction.Claims = nil
	e.Verification.Verified = nil
	e.Documents[1].Partial = true
	e.Documents[0].Partial = true
	prior, _ := json.Marshal(first[0].Offers)
	got, err := mergeAI(first, e, &code)
	if err != nil || len(got) != 0 || code != "ai_no_change" {
		t.Fatal(got, err, code, string(prior))
	}
}
