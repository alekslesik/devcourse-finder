package updater

import (
	"testing"
	"time"
)

func TestAISkyproScopeAndBindings(t *testing.T) {
	raw, c := skyFixture(t, "python")
	r, o, e := collectSkypro(raw, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundReader(t, raw, c, r, o)
}
func TestAISkyproPostgres(t *testing.T) {
	raw, c := skyFixture(t, "python")
	r, o, e := collectSkypro(raw, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundPostgres(t, raw, c, r, o)
}
