package updater

import (
	"testing"
	"time"
)

func TestAIOtusScopeAndBindings(t *testing.T) {
	raw := fixtureHTML(t, "otus-python-schema.json")
	c := Candidate{Adapter: "otus", ExternalID: "python-basic", URL: "https://otus.ru/lessons/python-basic"}
	r, o, e := collectCandidate(raw, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundReader(t, raw, c, r, o)
}
func TestAIOtusPostgres(t *testing.T) {
	raw := fixtureHTML(t, "otus-python-schema.json")
	c := Candidate{Adapter: "otus", ExternalID: "python-basic", URL: "https://otus.ru/lessons/python-basic"}
	r, o, e := collectCandidate(raw, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundPostgres(t, raw, c, r, o)
}
