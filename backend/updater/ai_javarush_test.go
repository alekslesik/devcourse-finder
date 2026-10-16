package updater

import (
	"testing"
	"time"
)

func TestAIJavaRushScopeAndBindings(t *testing.T) {
	raw, p, c := jrFixture(t)
	r, o, e := collectJavaRush(raw, p, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundReader(t, raw, c, r, o)
}
func TestAIJavaRushPostgres(t *testing.T) {
	raw, p, c := jrFixture(t)
	r, o, e := collectJavaRush(raw, p, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundPostgres(t, raw, c, r, o)
}
