package updater

import (
	"testing"
	"time"
)

func TestAISkillfactoryScopeAndBindings(t *testing.T) {
	raw, p, c := sfFixture(t)
	r, o, e := collectSkillfactory(raw, p, c, time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundReader(t, raw, c, r, o)
}
func TestAISkillfactoryPostgres(t *testing.T) {
	raw, p, c := sfFixture(t)
	r, o, e := collectSkillfactory(raw, p, c, time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundPostgres(t, raw, c, r, o)
}
