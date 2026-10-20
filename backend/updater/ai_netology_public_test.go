package updater

import (
	"context"
	"devcourse-finder/catalog"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAINetologyPublicCurriculumIsPartialAndCannotBecomeAnOffer(t *testing.T) {
	_, c, current, b := aiNetologyFixture(t)
	raw, err := os.ReadFile("testdata/netology-public-go.html")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := aiNetologyDocuments(raw, c, current, b, fingerprint(raw), time.Now())
	if err != nil || len(docs) != 1 || docs[0].ID != "course-public" || !docs[0].Partial || docs[0].Scope != "course" || docs[0].OfferID != "" {
		t.Fatal(docs, err)
	}
	if strings.Contains(docs[0].Text, "Оплатите") || strings.Contains(docs[0].Text, "golpro") {
		t.Fatal("registration variants leaked")
	}
	// Course-only input cannot supply a tariff condition even with a real quote.
	x := AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{{DocumentID: docs[0].ID, Field: "weekly_workload", OfferID: b.OfferID, Quote: docs[0].Text[:100], Min: valueFloat(1), Max: valueFloat(5), Unit: "hours_per_week"}}}
	if _, err = aiCompile(c, docs, x, AIVerification{Complete: true, Verified: []int{0}}, time.Now()); err == nil {
		t.Fatal("course became offer evidence")
	}
	for _, bad := range []string{
		strings.Replace(string(raw), `"programName": "go"`, `"programName": "java"`, 1),
		strings.Replace(string(raw), `"slug": "go"`, `"slug": "other"`, 1),
		strings.Replace(string(raw), `<h1>Go-разработчик с нуля</h1>`, `<h1>Another course</h1>`, 1),
		strings.Replace(string(raw), `"landingContent":`, `"program":{"data":{"program_id":999}},"landingContent":`, 1),
		strings.Replace(string(raw), `rel="canonical" href="https://netology.ru/programs/go"`, `rel="canonical" href="https://netology.ru/programs/java"`, 1),
	} {
		if bad == string(raw) {
			t.Fatal("mutation missed")
		}
		if _, err = aiNetologyDocuments([]byte(bad), c, current, b, fingerprint([]byte(bad)), time.Now()); err == nil {
			t.Fatal("wrong identity accepted")
		}
	}
}

func TestAIPartialCourseInputPreservesPreviouslyVerifiedProfile(t *testing.T) {
	_, record, e := aiEventFixture(t)
	code := ""
	first, err := mergeAI([]catalog.Course{record}, e, &code)
	if err != nil || len(first) != 1 || first[0].Profile == nil {
		t.Fatal(first, err)
	}
	e.IdentityDigest = aiIdentityDigest(first[0])
	e.Extraction.Claims = nil
	e.Verification.Verified = nil
	// A partial profile document cannot attest absence of a previously seen claim.
	e.Documents = e.Documents[:1]
	e.Documents[0].Partial = true
	got, err := mergeAI(first, e, &code)
	if err != nil || len(got) != 0 || code != "ai_no_change" {
		t.Fatal(got, code, err)
	}
}

// Public projection content is augmented with one synthetic claim to test the
// plumbing, not the accuracy of a real model on the captured curriculum.
type publicNetologyModel struct{ calls int }

func (m *publicNetologyModel) Extract(_ context.Context, c Candidate, d []AIDocument) (AIExtraction, AIUsage, error) {
	m.calls++
	return AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{{Field: "audience", DocumentID: d[0].ID, Quote: "Программа предназначена для начинающих без опыта программирования.", Values: []string{"none"}}}}, AIUsage{Prompt: 100, Total: 100}, nil
}
func (m *publicNetologyModel) Verify(_ context.Context, _ Candidate, _ []AIDocument, _ AIExtraction) (AIVerification, AIUsage, error) {
	m.calls++
	return AIVerification{Complete: true, Verified: []int{0}}, AIUsage{Prompt: 50, Total: 50}, nil
}
func TestAINetologyPublicPipelineRejectsRemapAndPreservesAllOffers(t *testing.T) {
	db := postgresFixture(t)
	_, c, record, b := aiNetologyFixture(t)
	record.Profile = nil
	record.Audience = []string{"unknown"}
	record.Goals = []string{"unknown"}
	record.Direction = "unknown"
	extra := record.Offers[0]
	extra.ID += "-manual"
	extra.Name = "Manual"
	record.Offers = append(record.Offers, extra)
	raw, err := os.ReadFile("testdata/netology-public-go.html")
	if err != nil {
		t.Fatal(err)
	}
	raw = netologyMutate(t, raw, func(d map[string]any) {
		state := d["props"].(map[string]any)["pageProps"].(map[string]any)["initialState"].(map[string]any)
		content := state["landingContent"].(map[string]any)["content"].(map[string]any)
		for key, value := range content {
			if strings.HasPrefix(key, "programModule_") {
				blocks := value.(map[string]any)["blocks"].([]any)
				if len(blocks) > 0 {
					blocks[0].(map[string]any)["description"] = "Программа предназначена для начинающих без опыта программирования."
					break
				}
			}
		}
	})
	if !strings.Contains(string(raw), "Программа предназначена для начинающих без опыта программирования.") {
		t.Fatal("claim mutation missed")
	}
	ctx := context.Background()
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"netology.ru"}, Courses: []catalog.Course{record}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	before, err := db.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id,netology_family_id,netology_program_id) VALUES('netology',$1,$2,$3,$4,$5,$6)`, c.ExternalID, c.URL, record.ID, b.OfferID, b.FamilyID, b.ProgramID)
	if err != nil {
		t.Fatal(err)
	}
	model := &publicNetologyModel{}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(raw)))}, nil
	})}
	service := AIService{DB: db, Config: AIConfig{Model: "test", VerifyModel: "test-review", BatchSize: 1, DailyLimit: 2}, Model: model, Client: client}
	result, err := service.Run(ctx)
	if err != nil || result.Queued != 1 || result.Failed != 0 {
		t.Fatal(result, err)
	}
	publisher := Service{DB: db, Config: productionConfig(t), Worker: "publisher"}
	if _, err = db.Pool.Exec(ctx, `UPDATE catalog_identities SET netology_program_id=netology_program_id+1`); err != nil {
		t.Fatal(err)
	}
	rejected, err := publisher.Publish(ctx)
	if err != nil || rejected.RefreshedCourses != 0 {
		t.Fatal(rejected, err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE catalog_identities SET netology_program_id=$1`, b.ProgramID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE catalog_ai_sources SET next_attempt_at=now()`); err != nil {
		t.Fatal(err)
	}
	result, err = service.Run(ctx)
	if err != nil || result.Cached != 1 || model.calls != 2 {
		t.Fatal(result, err, model.calls)
	}
	if _, err = publisher.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := db.Load(ctx)
	if err != nil || after[0].Profile == nil || after[0].Audience[0] != "none" {
		t.Fatal(after, err)
	}
	if !reflect.DeepEqual(before[0].Offers, after[0].Offers) || !before[0].CheckedAt.Equal(after[0].CheckedAt) {
		t.Fatal("public metadata changed offer/financial freshness")
	}
}

// This check verifies public course scope only, using an existing-binding fixture;
// it does not claim live checkout ID verification or make model requests.
func TestAINetologyPublicLiveSource(t *testing.T) {
	if os.Getenv("AI_NETOLOGY_PUBLIC_SOURCE_CHECK") != "1" {
		t.Skip("explicit anonymous public-source check only")
	}
	_, c, current, b := aiNetologyFixture(t)
	client := NewClient()
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	raw, digest, err := fetch(ctx, client, candidateEndpoint(c))
	if err != nil {
		t.Fatal("source unavailable", rejectionCode(err, "source_unavailable"))
	}
	docs, err := aiNetologyDocuments(raw, c, current, b, digest, time.Now().UTC())
	if err != nil {
		t.Fatal("public curriculum contract rejected")
	}
	if len(docs) != 1 || !docs[0].Partial || docs[0].Scope != "course" {
		t.Fatal("unexpected scope")
	}
	t.Logf("scope=course partial=true bytes=%d; numeric checkout and offer conditions NOT verified", len(docs[0].Text))
}

func TestAINetologyPublicExcludesHiddenAndUnspecifiedModules(t *testing.T) {
	_, c, current, b := aiNetologyFixture(t)
	raw, err := os.ReadFile("testdata/netology-public-go.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []any{false, nil} {
		t.Run(fmt.Sprint(visible), func(t *testing.T) {
			bad := netologyMutate(t, raw, func(d map[string]any) {
				content := d["props"].(map[string]any)["pageProps"].(map[string]any)["initialState"].(map[string]any)["landingContent"].(map[string]any)["content"].(map[string]any)
				module := content["programModule_17635519407340"].(map[string]any)
				if visible == nil {
					delete(module, "isVisible")
				} else {
					module["isVisible"] = visible
				}
				module["blocks"] = []any{map[string]any{"title": "HIDDEN_CURRICULUM", "description": "HIDDEN_PROFILE_CLAIM"}}
			})
			docs, err := aiNetologyDocuments(bad, c, current, b, fingerprint(bad), time.Now())
			if err != nil {
				t.Fatal(err)
			}
			if len(docs) != 1 || strings.Contains(docs[0].Text, "HIDDEN_") {
				t.Fatal("hidden curriculum leaked", docs)
			}
		})
	}
}
