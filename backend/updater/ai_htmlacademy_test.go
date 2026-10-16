package updater

import (
	"testing"
	"time"
)

func TestAIAcademyScopeAndBindings(t *testing.T) {
	raw, p, c := academyFixture(t, "react")
	r, o, e := collectHTMLAcademy(raw, academyEnvelope(p), c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundReader(t, raw, c, r, o)
}
func TestAIAcademyPostgres(t *testing.T) {
	raw, p, c := academyFixture(t, "react")
	r, o, e := collectHTMLAcademy(raw, academyEnvelope(p), c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundPostgres(t, raw, c, r, o)
}
