package updater

import (
	"testing"
)

func TestAIPurpleScopeAndBindings(t *testing.T) {
	raw, c := purpleFixture(t, "python-start")
	r, o, e := collectPurpleSchool(raw, c, netologyNow())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundReader(t, raw, c, r, o)
}
func TestAIPurplePostgres(t *testing.T) {
	raw, c := purpleFixture(t, "python-start")
	r, o, e := collectPurpleSchool(raw, c, netologyNow())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundPostgres(t, raw, c, r, o)
}
