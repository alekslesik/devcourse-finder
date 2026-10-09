package updater

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func fixtureHTML(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(`<script type="application/ld+json">` + string(raw) + `</script>`)
}
func TestRecordedOtusFullPriceAndExpiredYandexOffer(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	c := Candidate{Adapter: "otus", ExternalID: "python-basic", URL: "https://otus.ru/lessons/python-basic"}
	record, o, err := collectCandidate(fixtureHTML(t, "otus-python-schema.json"), c, now)
	if err != nil || record.Language != "python" || o.Price == nil || *o.Price != 13000000 || o.Enrollment != "open" {
		t.Fatalf("official Otus contract rejected %#v %v", o, err)
	}
	yandex := Candidate{Adapter: "yandex", ExternalID: "backend-developer", URL: "https://practicum.yandex.ru/backend-developer"}
	if _, _, err = collectCandidate(fixtureHTML(t, "yandex-python-schema.json"), yandex, now); err == nil {
		t.Fatal("expired 2025 starting-price offer published")
	}
	raw, err := os.ReadFile("testdata/stepik-go.json")
	if err != nil {
		t.Fatal(err)
	}
	record, o, err = collectCandidate(raw, Candidate{Adapter: "stepik", ExternalID: "54403", URL: "https://stepik.org/course/54403/promo"}, now)
	if err != nil || record.Language != "go" || o.Price == nil || *o.Price != 0 {
		t.Fatal("official Stepik detail rejected", err)
	}
}
func TestStructuredProvidersRejectIntroInstallmentsAndRecommendations(t *testing.T) {
	now := time.Now()
	canonicalURL := "https://ru.hexlet.io/programs/java"
	node := map[string]any{"@type": "Course", "url": canonicalURL, "name": "Java", "description": "Курс Java", "offers": map[string]any{"@type": "Offer", "price": 85000, "priceCurrency": "RUB", "availability": "https://schema.org/InStock"}}
	c := Candidate{Adapter: "hexlet", ExternalID: "java", URL: canonicalURL}
	html := func() []byte {
		b, _ := json.Marshal(node)
		return []byte(`<script type="application/ld+json">` + string(b) + `</script>`)
	}
	if _, o, err := collectCandidate(html(), c, now); err != nil || o.Price != nil || !o.PriceUnknown {
		t.Fatal("unverified provider amount advertised as full price", err)
	}
	offer := node["offers"].(map[string]any)
	offer["priceSpecification"] = map[string]any{"billingDuration": "P1M"}
	if _, _, err := collectCandidate(html(), c, now); err == nil {
		t.Fatal("installment accepted")
	}
	delete(offer, "priceSpecification")
	offer["price"] = 0
	if _, _, err := collectCandidate(html(), c, now); err == nil {
		t.Fatal("intro price accepted as free course")
	}
	node["isAccessibleForFree"] = true
	node["url"] = "https://ru.hexlet.io/programs/unrelated"
	if _, _, err := collectCandidate(html(), c, now); err == nil {
		t.Fatal("unrelated recommendation accepted")
	}
	if _, _, err := collectCandidate([]byte("<html>Access denied</html>"), c, now); err == nil {
		t.Fatal("challenge published")
	}
}
func TestUnknownCurrencyAndAggregateAreNotExactPrices(t *testing.T) {
	body := `<script type="application/ld+json">{"@type":"Course","url":"https://otus.ru/lessons/java-basic","name":"Java","description":"Java course","offers":{"@type":"Offer","price":100,"priceCurrency":"USD","availability":"https://schema.org/InStock"}}</script>`
	c := Candidate{Adapter: "otus", ExternalID: "java-basic", URL: "https://otus.ru/lessons/java-basic"}
	_, o, err := collectCandidate([]byte(body), c, time.Now())
	if err != nil || o.Price != nil || !o.PriceUnknown {
		t.Fatal("USD became RUB", err)
	}
	aggregate := strings.Replace(strings.Replace(body, `"Offer"`, `"AggregateOffer"`, 1), `"price":100`, `"lowPrice":100`, 1)
	_, o, err = collectCandidate([]byte(aggregate), c, time.Now())
	if err != nil || o.Price != nil || !o.PriceUnknown {
		t.Fatal("starting price became exact", err)
	}
}

func TestHexletDetailUsesVerifiedCanonicalPath(t *testing.T) {
	c := Candidate{Adapter: "hexlet", ExternalID: "python", URL: "https://ru.hexlet.io/programs/python"}
	if candidateEndpoint(c) != c.URL {
		t.Fatal("Hexlet trailing slash redirects before verification")
	}
}
