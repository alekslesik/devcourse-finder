package updater

import (
	"testing"
	"time"
)

func TestAISkillboxScopeAndBindings(t *testing.T) {
	raw, c := skillboxFixture(t)
	r, o, e := collectSkillbox(raw, c, time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundReader(t, raw, c, r, o)
}
func TestAISkillboxPostgres(t *testing.T) {
	raw, c := skillboxFixture(t)
	r, o, e := collectSkillbox(raw, c, time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	aiCheckBoundPostgres(t, raw, c, r, o)
}
