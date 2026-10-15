package updater

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func aiBasicsFixture(t *testing.T) ([]byte, []byte, Candidate, catalog.Course) {
	t.Helper()
	raw, err := os.ReadFile("testdata/codebasics-python-page.json")
	if err != nil {
		t.Fatal(err)
	}
	var page map[string]any
	if json.Unmarshal(raw, &page) != nil {
		t.Fatal("fixture")
	}
	props := page["props"].(map[string]any)
	landing := props["courseLandingPage"].(map[string]any)
	landing["description"] = "Курс разработан для начинающих без опыта программирования. Научитесь создавать программы на Python и разберётесь в основах языка."
	landing["used_in_description"] = "PUBLIC_USAGE_ONLY: Python применяется для разработки серверных приложений."
	landing["outcomes_description"] = "PUBLIC_OUTCOME_ONLY: Вы научитесь писать небольшие программы самостоятельно."
	landing["duration"] = "27 часов"
	props["auth"] = map[string]any{"email": "AUTH_ONLY_DO_NOT_SEND"}
	props["landingPagesForFooter"] = []any{map[string]any{"description": "OTHER_COURSE_ONLY"}}
	raw, err = json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	body := basicsHTML(string(raw))
	policy := fixtureHTML(t, "codebasics-free-policy.json")
	c := Candidate{Adapter: "codebasics", ExternalID: "python", URL: "https://code-basics.com/ru/languages/python"}
	current, _, err := collectCodeBasics(body, policy, c, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return body, policy, c, current
}

func TestAICodeBasicsSelectsPublicFieldsAndOnlyPrimaryOffer(t *testing.T) {
	raw, policy, c, current := aiBasicsFixture(t)
	extra := current.Offers[0]
	extra.ID = "manual-extra"
	extra.Name = "Manual"
	current.Offers = append(current.Offers, extra)
	docs, err := aiCodeBasicsDocuments(raw, policy, c, current, current.Offers[0].ID, fingerprint(raw), time.Now().UTC())
	if err != nil || len(docs) != 2 {
		t.Fatal(docs, err)
	}
	if !strings.Contains(docs[0].Text, "PUBLIC_USAGE_ONLY") || !strings.Contains(docs[0].Text, "PUBLIC_OUTCOME_ONLY") || !strings.Contains(docs[1].Text, "27 часов") {
		t.Fatal("public description lost", docs)
	}
	for _, d := range docs {
		if strings.Contains(d.Text, "AUTH_ONLY") || strings.Contains(d.Text, "OTHER_COURSE_ONLY") || d.OfferID == extra.ID {
			t.Fatal("private or unrelated fields leaked", d)
		}
	}
}

func TestAICodeBasicsRejectsSourceIdentityPolicyAndBindingChanges(t *testing.T) {
	for _, kind := range []string{"policy", "course", "offer", "language", "title", "draft", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			raw, policy, c, current := aiBasicsFixture(t)
			offerID := current.Offers[0].ID
			switch kind {
			case "policy":
				policy = nil
			case "course":
				current.Source = "https://code-basics.com/ru/languages/java"
			case "offer":
				offerID = "another-offer"
			case "language":
				current.Language = "java"
			case "title":
				current.Title = "Another course"
			case "draft":
				raw = []byte(strings.Replace(string(raw), `"state":"published"`, `"state":"draft"`, 1))
			case "oversize":
				raw = []byte(strings.Replace(string(raw), "PUBLIC_USAGE_ONLY", strings.Repeat("x", aiMaxText+1), 1))
			}
			if _, err := aiCodeBasicsDocuments(raw, policy, c, current, offerID, fingerprint(raw), time.Now().UTC()); err == nil {
				t.Fatal("unsafe input accepted", kind)
			}
		})
	}
}

func TestAICodeBasicsLiveSource(t *testing.T) {
	if os.Getenv("AI_CODEBASICS_SOURCE_CHECK") != "1" {
		t.Skip("explicit anonymous source check only")
	}
	c, ok := aiCandidateURL("https://code-basics.com/ru/languages/python")
	if !ok {
		t.Fatal("source unsupported")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	client := NewClient()
	defer client.CloseIdleConnections()
	raw, digest, err := fetch(ctx, client, candidateEndpoint(c))
	if err != nil {
		t.Fatal("course source unavailable")
	}
	policy, _, err := fetch(ctx, client, "https://code-basics.com/ru")
	if err != nil {
		t.Fatal("policy source unavailable")
	}
	at := time.Now().UTC()
	current, _, err := collectCodeBasics(raw, policy, c, at)
	if err != nil {
		t.Fatal("native source contract rejected")
	}
	docs, err := aiCodeBasicsDocuments(raw, policy, c, current, current.Offers[0].ID, digest, at)
	if err != nil {
		t.Fatal("AI documents rejected")
	}
	for _, d := range docs {
		t.Logf("source_scope=%s bytes=%d segments=%d", d.Scope, len(d.Text), len(aiQuoteSegments(d.Text)))
	}
}
