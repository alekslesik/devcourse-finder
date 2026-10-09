package updater

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func basicsFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/codebasics-python-page.json")
	if err != nil {
		t.Fatal(err)
	}
	return basicsHTML(string(raw))
}
func basicsHTML(raw string) []byte {
	return []byte(`<link rel="canonical" href="https://code-basics.com/ru/languages/python"><script data-page="app" type="application/json">` + raw + `</script><a href="/ru/languages/python/lessons/hello-world">Начать обучение</a>`)
}
func TestRecordedCodeBasicsCompleteProgramAndFreePolicy(t *testing.T) {
	c := Candidate{Adapter: "codebasics", ExternalID: "python", URL: "https://code-basics.com/ru/languages/python"}
	policy := fixtureHTML(t, "codebasics-free-policy.json")
	record, o, err := collectCodeBasics(basicsFixture(t), policy, c, time.Now())
	if err != nil || record.Title != "Курс Python" || o.Price == nil || *o.Price != 0 || o.Enrollment != "continuous" || !record.Offers[0].Free {
		t.Fatal(record, o, err)
	}
	if _, _, err = collectCodeBasics(basicsFixture(t), nil, c, time.Now()); err == nil {
		t.Fatal("missing price policy became free")
	}
	contradictory := []byte(strings.ReplaceAll(string(policy), "участие в его разработке", "участие в его разработке. Теперь все курсы платные"))
	if _, _, err = collectCodeBasics(basicsFixture(t), contradictory, c, time.Now()); err == nil {
		t.Fatal("contradictory pricing policy accepted")
	}
	changed := []byte(strings.ReplaceAll(string(policy), "бесплатно", "за 100 рублей"))
	if _, _, err = collectCodeBasics(basicsFixture(t), changed, c, time.Now()); err == nil {
		t.Fatal("paid policy became free")
	}
}
func TestCodeBasicsRejectsWrongIdentityDraftUnbuiltAndPreview(t *testing.T) {
	c := Candidate{Adapter: "codebasics", ExternalID: "python", URL: "https://code-basics.com/ru/languages/python"}
	policy := fixtureHTML(t, "codebasics-free-policy.json")
	raw, err := os.ReadFile("testdata/codebasics-python-page.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*basicsPage){
		func(p *basicsPage) { p.URL = "/ru/languages/java" },
		func(p *basicsPage) { p.Props.Landing.State = "draft" },
		func(p *basicsPage) { p.Props.Landing.Main = false },
		func(p *basicsPage) { p.Props.Landing.Listed = false },
		func(p *basicsPage) { p.Props.Course.Version.State = "building" },
		func(p *basicsPage) { p.Props.Course.Version.Result = "Failed" },
		func(p *basicsPage) { p.Props.Lessons["14"] = p.Props.Lessons["14"][:1] },
		func(p *basicsPage) { p.Props.Lessons["14"][0].Language.Slug = "java" },
		func(p *basicsPage) { p.Props.Locale = "en" },
	} {
		var page basicsPage
		if json.Unmarshal(raw, &page) != nil {
			t.Fatal("fixture")
		}
		change(&page)
		body, _ := json.Marshal(page)
		if _, _, err = collectCodeBasics(basicsHTML(string(body)), policy, c, time.Now()); err == nil {
			t.Fatal("unsupported page published", string(body))
		}
	}
	if _, _, err = collectCodeBasics([]byte(`<html>Login required</html>`), policy, c, time.Now()); err == nil {
		t.Fatal("challenge published")
	}
	body := basicsFixture(t)
	if _, _, err = collectCodeBasics(append(append([]byte{}, body...), body...), policy, c, time.Now()); err == nil {
		t.Fatal("duplicate identity payload accepted")
	}
}
