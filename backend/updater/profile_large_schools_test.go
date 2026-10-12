package updater

import (
	"devcourse-finder/catalog"
	"os"
	"testing"
	"time"
)

func TestLargeSchoolProfilesUseBoundSections(t *testing.T) {
	for _, adapter := range []string{"yandex", "skillbox", "skillfactory", "skypro"} {
		raw, err := os.ReadFile("testdata/profiles/" + adapter + ".html")
		if err != nil {
			t.Fatal(err)
		}
		c := Candidate{Adapter: adapter, URL: "https://" + providerHosts[adapter] + "/course"}
		r, o := attachHTMLProfile(raw, c, catalog.Course{Language: "python"}, Observation{}, time.Now().UTC())
		if !o.ProfileObserved || !r.Profile.Valid() {
			t.Fatal(adapter, r.Profile)
		}
		if adapter == "skillfactory" && (r.Profile == nil || r.Profile.Audience == nil) {
			t.Fatal("recorded explicit course audience lost")
		}
		if adapter == "skillbox" && r.Profile != nil && r.Profile.Audience != nil {
			t.Fatal("general language advertising became target audience")
		}
	}
	// Explicit synthetic mutation: unrelated courses and job requirements cannot leak.
	raw := []byte(`<section class="related-course"><h2>Кому подойдет курс</h2><p>Курс для опытных разработчиков</p></section><section><h2>Требования:</h2><p>Опыт коммерческой разработки</p></section>`)
	c := Candidate{Adapter: "skillfactory", URL: "https://skillfactory.ru/python-developer"}
	r, _ := attachHTMLProfile(raw, c, catalog.Course{Language: "python"}, Observation{}, time.Now().UTC())
	if r.Profile != nil {
		t.Fatal("unrelated evidence", r.Profile)
	}
}

func TestLargeSchoolProfileContractMutations(t *testing.T) {
	// Synthetic contract mutations, not recorded live coverage.
	for _, adapter := range []string{"yandex", "netology", "skillbox", "skillfactory", "skypro"} {
		h := profileContracts[adapter]
		raw := []byte("<section><h2>" + h.Audience[0] + "</h2><p>Для начинающих без опыта программирования.</p></section>")
		c := Candidate{Adapter: adapter, URL: "https://" + providerHosts[adapter] + "/course"}
		p := profileFromSections(raw, c, "python", time.Now().UTC(), h)
		if p.Audience == nil || len(p.Audience.Values) != 1 || p.Audience.Values[0] != "none" {
			t.Fatal(adapter, p)
		}
	}
}

func TestAllHTMLProviderProfileFieldsRequireSemanticSections(t *testing.T) {
	for adapter, h := range profileContracts {
		// Synthetic positive contract mutations: no claim about current live coverage.
		raw := []byte("<section><h2>" + h.Audience[0] + "</h2><p>Для начинающих без опыта.</p></section><section><h2>" + h.Outcomes[0] + "</h2><p>Вы сможете подготовиться к трудоустройству и разрабатывать серверные приложения. Целевой уровень: самостоятельные проекты.</p></section>")
		c := Candidate{Adapter: adapter, URL: "https://" + providerHosts[adapter] + "/course"}
		p := profileFromSections(raw, c, "python", time.Now().UTC(), h)
		if p.Audience == nil || p.Goals == nil || p.Direction == nil || p.TargetLevel == nil || !p.Valid() {
			t.Fatal(adapter, p)
		}
	}
}
