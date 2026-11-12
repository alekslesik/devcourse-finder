package updater

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func currentHexletFixture(t *testing.T, slug string) ([]byte, Candidate, catalog.Course) {
	t.Helper()
	_, c, r := hexletPublicFixture(t, slug)
	file := map[string]string{"java": "hexlet-java", "frontend": "hexlet-frontend", "java-basics-free": "hexlet-java-free", "javascript-basics-free": "hexlet-javascript-free"}[slug]
	raw, err := os.ReadFile("testdata/current-schools/" + file + ".html")
	if err != nil {
		t.Fatal(err)
	}
	return raw, c, r
}

func TestAIHexletCurrentOwnSchemaFAQAndPaymentIdentity(t *testing.T) {
	for _, slug := range []string{"java", "frontend", "java-basics-free", "javascript-basics-free"} {
		t.Run(slug, func(t *testing.T) {
			raw, c, r := currentHexletFixture(t, slug)
			docs, err := aiHexletCurrent(raw, c, r, nil, fingerprint(raw))
			if err != nil || len(docs) != 1 || !docs[0].Partial || docs[0].Scope != "course" || !aiHexletPublicBundle(docs, r.ID) {
				t.Fatal(docs, err)
			}
			if !strings.Contains(docs[0].Text, "программирован") && !strings.Contains(docs[0].Text, "Программирован") {
				t.Fatal("own suitability text lost", docs[0])
			}
			promoted := strings.Replace(string(raw), "</body>", `<section><h2>Платная профессия</h2><p>FOREIGN_PROMOTION: персональный наставник, 10 месяцев, гарантированная работа</p><a href="https://another.example/pay">Купить</a></section></body>`, 1)
			same, err := aiHexletCurrent([]byte(promoted), c, r, nil, fingerprint([]byte(promoted)))
			if err != nil || same[0].Text != docs[0].Text {
				t.Fatal("another product entered course scope", same, err)
			}
			for _, bad := range []string{
				strings.Replace(string(raw), "</body>", "<h1>Другой курс</h1></body>", 1),
				strings.ReplaceAll(string(raw), "https://ru.hexlet.io/programs/"+slug, "https://ru.hexlet.io/programs/other"),
				strings.Replace(string(raw), "</body>", `<script type="application/ld+json">{"@type":"Course","url":"https://ru.hexlet.io/programs/other"}</script></body>`, 1),
				strings.Replace(string(raw), "</body>", `<script type="application/ld+json">{"@type":"Product","name":"Other"}</script></body>`, 1),
				strings.Replace(string(raw), "</body>", `<script type="application/ld+json">{"@type":"Offer"}</script></body>`, 1),
			} {
				if _, err := aiHexletCurrent([]byte(bad), c, r, nil, fingerprint([]byte(bad))); err == nil {
					t.Fatal("foreign or ambiguous identity accepted", slug)
				}
			}
			if slug == "java" || slug == "frontend" {
				for _, suffix := range []string{"from=other", "from=tilda_page&amp;from=other"} {
					bad := strings.ReplaceAll(string(raw), "from=tilda_page", suffix)
					if _, err := aiHexletCurrent([]byte(bad), c, r, nil, fingerprint([]byte(bad))); err == nil {
						t.Fatal("unbound payment source marker accepted")
					}
				}
			} else {
				bad := strings.ReplaceAll(string(raw), "/goto", "/other")
				if _, err := aiHexletCurrent([]byte(bad), c, r, nil, fingerprint([]byte(bad))); err == nil {
					t.Fatal("unbound free start accepted")
				}
			}
		})
	}
}

func TestAIHexletRecommendationOnlyRetainsTariffBundleIdentity(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	_, c, r := hexletPublicFixture(t, "java")
	r.Direction = "unknown"
	r.Profile = nil
	if err := db.Import(ctx, catalog.Dataset{Domains: []string{"ru.hexlet.io"}, Courses: []catalog.Course{r}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	before, err := db.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r = before[0]
	raw, err := os.ReadFile("testdata/hexlet-public-java-tariffs.html")
	if err != nil {
		t.Fatal(err)
	}
	docs, err := aiHexletPublic(raw, c, r, nil, fingerprint(raw))
	if err != nil || len(docs) != 4 {
		t.Fatal(docs, err)
	}
	courseDocs := aiRecommendationDocuments(docs)
	if len(courseDocs) != 1 {
		t.Fatal(courseDocs)
	}
	quote := []rune(courseDocs[0].Text)
	if len(quote) > 500 {
		quote = quote[:500]
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	e := AIEvent{Version: 1, RecommendationsOnly: true, CourseID: r.ID, IdentityDigest: aiIdentityDigest(r), Candidate: c, ObservedAt: now, Model: "deepseek:test", Policy: aiEffectivePolicy(), Documents: courseDocs, Extraction: AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{{Field: "recommended_directions", DocumentID: courseDocs[0].ID, Quote: string(quote), Values: []string{"backend"}}}}, Verification: AIVerification{Complete: true, Verified: []int{0}}}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	literal := e
	literal.RecommendationsOnly = false
	if ok, err := aiBoundEventIdentityValid(ctx, tx, literal); err != nil || ok {
		t.Fatal("literal event bypassed the complete bundle requirement", ok, err)
	}
	if ok, err := aiBoundEventIdentityValid(ctx, tx, e); err != nil || !ok {
		t.Fatal("filtered recommendation bundle lost identity", ok, err)
	}
	tx.Rollback(ctx)
	s := Service{DB: db}
	if err = s.enqueueAI(ctx, e); err != nil {
		t.Fatal(err)
	}
	if n, err := s.publishAI(ctx); err != nil || n != 1 {
		var code string
		_ = db.Pool.QueryRow(ctx, "SELECT code FROM catalog_ai_observations ORDER BY id DESC LIMIT 1").Scan(&code)
		t.Fatal(n, err, code)
	}
	after, err := db.Load(ctx)
	if err != nil || after[0].Recommendations == nil || aiRecommendationProtectedDigest(before[0]) != aiRecommendationProtectedDigest(after[0]) {
		t.Fatal("publication changed protected facts", err)
	}
	// Persisted fields, offers and ownership remain exactly the imported values.
	after[0].Recommendations = before[0].Recommendations
	after[0].RecommendationIdentity = before[0].RecommendationIdentity
	if !reflect.DeepEqual(before, after) {
		t.Fatal("advice altered imported data")
	}
}

func TestAIHexletCurrentPaymentMarkerIsNarrow(t *testing.T) {
	c := Candidate{Adapter: "hexlet", ExternalID: "java", URL: "https://ru.hexlet.io/programs/java"}
	own := "https://ru.hexlet.io/payments/new?program=java&variant=standard"
	if aiHexletPaymentVariant(own+"&from=tilda_page", c) != "base" {
		t.Fatal("current own CTA rejected")
	}
	for _, url := range []string{own + "&from=evil", own + "&from=tilda_page&from=evil", own + "&from=tilda_page&price=other", strings.Replace(own, "program=java", "program=frontend", 1) + "&from=tilda_page"} {
		if aiHexletPaymentVariant(url, c) != "" {
			t.Fatal("foreign CTA accepted", url)
		}
	}
}
