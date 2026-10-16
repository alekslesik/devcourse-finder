package updater

import (
	"testing"
	"time"
)

func TestAIRSSchoolScopeAndBindings(t *testing.T) {
	raw, p, c := rsFixture(t, "javascript")
	r, o, e := collectRSSchool(raw, p, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundReader(t, raw, c, r, o)
}
func TestAIRSSchoolPostgres(t *testing.T) {
	raw, p, c := rsFixture(t, "javascript")
	r, o, e := collectRSSchool(raw, p, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundPostgres(t, raw, c, r, o)
}
