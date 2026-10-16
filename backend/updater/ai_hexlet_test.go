package updater

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"testing"
	"time"
)

func aiHexletFixture(t *testing.T) ([]byte, Candidate, catalog.Course, Observation) {
	t.Helper()
	c := Candidate{Adapter: "hexlet", ExternalID: "python", URL: "https://ru.hexlet.io/programs/python"}
	node := map[string]any{"@type": "Course", "url": c.URL, "name": "Python-разработчик", "description": "Профессия Python-разработчик: программа обучения разработке приложений на Python.", "offers": map[string]any{"@type": "Offer", "name": "Стандартный", "price": 100, "priceCurrency": "RUB", "availability": "https://schema.org/InStock"}}
	b, _ := json.Marshal(node)
	raw := []byte(`<html><body><script type="application/ld+json">` + string(b) + `</script><div class="tn-molecule">Стандартный <p>10 месяцев обучения</p></div></body></html>`)
	r, o, e := collectCandidate(raw, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	return raw, c, r, o
}
func TestAIHexletScopeAndBindings(t *testing.T) {
	raw, c, r, o := aiHexletFixture(t)
	aiCheckBoundReader(t, raw, c, r, o)
}
func TestAIHexletPostgres(t *testing.T) {
	raw, c, r, o := aiHexletFixture(t)
	aiCheckBoundPostgres(t, raw, c, r, o)
}
