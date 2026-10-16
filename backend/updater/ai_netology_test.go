package updater

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func aiNetologyFixture(t *testing.T) ([]byte, Candidate, catalog.Course, aiNetologyBinding) {
	t.Helper()
	raw, c := netologyFixture(t, "go")
	// Add a course-level source section outside the captured real plan cards.
	section := `<section><h2>Кому подойдёт курс</h2><p>Программа предназначена для начинающих без опыта программирования. Специальные навыки не нужны, обучение начинается с нуля. Вы будете изучать разработку серверных приложений на Go и выполнять задания с подробными объяснениями.</p></section>`
	raw = []byte(strings.Replace(string(raw), "<body>", "<body>"+section, 1))
	record, o, err := collectNetology(raw, c, netologyNow())
	if err != nil {
		t.Fatal(err)
	}
	return raw, c, record, aiNetologyBinding{OfferID: record.Offers[0].ID, FamilyID: o.NetologyFamilyID, ProgramID: o.NetologyProgramID}
}

func TestAINetologyKeepsOnlyBoundTariffAndWholeCourseText(t *testing.T) {
	raw, c, current, binding := aiNetologyFixture(t)
	current.Offers = append(current.Offers, catalog.Offer{ID: "manual-plus", Name: "Plus", URL: c.URL})
	raw = []byte(strings.Replace(string(raw), "9 месяцев и 278 часов практики", "BASE_ONLY: 9 месяцев и 278 часов практики", 1))
	raw = []byte(strings.Replace(string(raw), `name="sections[0].cards[2].features[0].title">`, `name="sections[0].cards[2].features[0].title">OTHER_PROGRAM_ONLY: `, 1))
	docs, err := aiNetologyDocuments(raw, c, current, binding, fingerprint(raw), netologyNow())
	if err != nil || len(docs) != 2 {
		t.Fatal(docs, err)
	}
	for _, d := range docs {
		if strings.Contains(d.Text, "OTHER_PROGRAM_ONLY") {
			t.Fatal("another program leaked", d)
		}
		if d.Scope == "offer" && (d.OfferID != binding.OfferID || !strings.Contains(d.Text, "BASE_ONLY")) {
			t.Fatal("wrong tariff", d)
		}
		if d.Scope == "course" && (!strings.Contains(d.Text, "Специальные навыки не нужны") || strings.Contains(d.Text, "BASE_ONLY")) {
			t.Fatal("course text cropped or mixed with tariff", d)
		}
	}
}

func TestAINetologyRejectsChangedIdentityAndUnboundOffers(t *testing.T) {
	for _, kind := range []string{"program", "family", "language", "title", "offer_id", "offer_url", "offer_name", "canonical", "page_identity", "oversize", "ambiguous_plan"} {
		t.Run(kind, func(t *testing.T) {
			raw, c, current, binding := aiNetologyFixture(t)
			switch kind {
			case "program":
				binding.ProgramID++
			case "family":
				binding.FamilyID++
			case "language":
				current.Language = "python"
			case "title":
				current.Title = "Another program"
			case "offer_id":
				binding.OfferID = "other"
			case "offer_url":
				current.Offers[0].URL = "https://netology.ru/programs/other"
			case "offer_name":
				current.Offers[0].Name = "Another plan"
			case "canonical":
				raw = []byte(strings.Replace(string(raw), `rel="canonical" href="https://netology.ru/programs/go"`, `rel="canonical" href="https://netology.ru/programs/python"`, 1))
			case "page_identity":
				raw = netologyMutate(t, raw, func(d map[string]any) { netologyProgramMap(d)["program_family_url"] = "other" })
			case "ambiguous_plan":
				raw = []byte(strings.Replace(string(raw), "</body>", `<p name="sections[0].cards[99].title">Unknown plan</p></body>`, 1))
			case "oversize":
				raw = []byte(strings.Replace(string(raw), "<body>", "<body><p>"+strings.Repeat("Курс без опыта. ", 10000)+"</p>", 1))
			}
			if _, err := aiNetologyDocuments(raw, c, current, binding, fingerprint(raw), netologyNow()); err == nil {
				t.Fatal("unsafe source accepted", kind)
			}
		})
	}
}

func TestAINetologyReadDoesNotAlterFinancialData(t *testing.T) {
	raw, c, current, binding := aiNetologyFixture(t)
	oldPrice := *current.Offers[0].Price
	// The fresh source still passes its own full-payment checks, while the
	// catalog's price remains untouched by the AI document builder.
	*current.Offers[0].Price = oldPrice + 100
	before := current.Offers[0].PriceCheckedAt
	if _, err := aiNetologyDocuments(raw, c, current, binding, fingerprint(raw), netologyNow().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if *current.Offers[0].Price != oldPrice+100 || !current.Offers[0].PriceCheckedAt.Equal(before) {
		t.Fatal("financial metadata modified")
	}
}

func TestAINetologyBindingParticipatesInDocumentValidationAndCacheKey(t *testing.T) {
	raw, c, current, binding := aiNetologyFixture(t)
	docs, err := aiNetologyDocuments(raw, c, current, binding, fingerprint(raw), netologyNow())
	if err != nil {
		t.Fatal(err)
	}
	original := aiBundleDigest(docs)
	old := docs[0].BindingDigest
	docs[0].BindingDigest = ""
	if aiDocumentsValid(c, docs) {
		t.Fatal("missing binding accepted")
	}
	docs[0].BindingDigest = fingerprint([]byte("different binding"))
	if aiDocumentsValid(c, docs) {
		t.Fatal("mixed binding accepted")
	}
	if aiBundleDigest(docs) == original {
		t.Fatal("cache reused under changed binding")
	}
	docs[0].BindingDigest = old
	if !aiDocumentsValid(c, docs) {
		t.Fatal("valid binding rejected")
	}
}

// Explicit source-only check: no model calls, database writes or publication.
func TestAINetologyLiveSource(t *testing.T) {
	if os.Getenv("AI_NETOLOGY_SOURCE_CHECK") != "1" {
		t.Skip("explicit anonymous source check only")
	}
	c, ok := aiCandidateURL("https://netology.ru/programs/go")
	if !ok {
		t.Fatal("unsupported source")
	}
	client := NewClient()
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	raw, digest, err := fetch(ctx, client, candidateEndpoint(c))
	if err != nil {
		t.Fatal("official source unavailable", rejectionCode(err, "source_unavailable"))
	}
	at := time.Now().UTC()
	current, o, err := collectNetology(raw, c, at)
	if err != nil {
		t.Fatal("native source contract", rejectionCode(err, "source_contract_rejected"))
	}
	docs, err := aiNetologyDocuments(raw, c, current, aiNetologyBinding{OfferID: current.Offers[0].ID, FamilyID: o.NetologyFamilyID, ProgramID: o.NetologyProgramID}, digest, at)
	if err != nil {
		t.Fatal("AI document contract rejected")
	}
	for _, d := range docs {
		t.Logf("source_scope=%s bytes=%d segments=%d", d.Scope, len(d.Text), len(aiQuoteSegments(d.Text)))
	}
}

func TestAINetologyCurrentRegistrationOnlyTemplateStaysUnverified(t *testing.T) {
	_, c, current, binding := aiNetologyFixture(t)
	raw := []byte(`<html lang="ru-RU"><head><link rel="canonical" href="https://netology.ru/programs/go"><script id="__NEXT_DATA__" type="application/json">{"props":{"pageProps":{"initialState":{"landingContent":{"content":{"programRegistration":{"isBreedingPrograms":true,"customBreedingPrograms":[{"slug":"go"},{"slug":"golpro"}]}}}}}},"query":{"programName":"go"}}</script></head><body><h1>Go-разработчик с нуля</h1><p>Описание программ и выбор варианта обучения не подтверждают идентичность продукта и его тарифа.</p></body></html>`)
	if _, err := aiNetologyDocuments(raw, c, current, binding, fingerprint(raw), time.Now().UTC()); err == nil {
		t.Fatal("registration-only page guessed product/tariff identity")
	}
	if candidateEndpoint(c) != "https://netology.ru/programs/go/" || c.URL != "https://netology.ru/programs/go" {
		t.Fatal("fetch/canonical URL contract changed", c)
	}
}

// The current live registration-only archive has neither a verified product nor
// family. Never promote it to a purchasable tier based on URL/title alone.
func TestAINetologyRejectsRegistrationOnlyTemplate(t *testing.T) {
	raw, c, current, binding := aiNetologyFixture(t)
	raw = netologyMutate(t, raw, func(d map[string]any) {
		delete(d, "page")
		delete(d, "isFallback")
		props := d["props"].(map[string]any)["pageProps"].(map[string]any)
		delete(props, "isActive")
		state := props["initialState"].(map[string]any)
		delete(state, "program")
		landing := state["landingContent"].(map[string]any)
		delete(landing, "programFamilyUrl")
		delete(landing, "templateType")
		landing["content"] = map[string]any{"programRegistration_1": map[string]any{"isVisible": true, "isBreedingPrograms": true, "customBreedingPrograms": []any{map[string]any{"slug": "go"}, map[string]any{"slug": "golpro"}}}}
	})
	if _, e := aiNetologyDocuments(raw, c, current, binding, fingerprint(raw), netologyNow()); e == nil {
		t.Fatal("registration-only archive accepted")
	}
}
