package catalog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func learningTestEvidence() LearningEvidence {
	return LearningEvidence{Source: "https://school.example/course", Locator: "course.weekly_workload", Digest: strings.Repeat("a", 64), CheckedAt: time.Now().UTC()}
}
func TestLearningFactsKeepRangeAndIndependentSupport(t *testing.T) {
	e := learningTestEvidence()
	o := Offer{SupportKnown: ptr(false), LearningConditions: &LearningConditions{WeeklyWorkload: &LearningRange{Min: 8, Max: 12, Unit: "hours_per_week", Evidence: e}, Review: &LearningSupport{Available: true, Evidence: e}}}
	if !o.LearningConditions.Valid() || o.WithinWeeklyHours(10) || !o.WithinWeeklyHours(12) || !o.MatchesSupport("review") || o.MatchesSupport("mentor") || o.MatchesSupport("self") {
		t.Fatal("unknown facts or workload range were inferred", o)
	}
	o.LearningConditions.Review.Available = false
	o.LearningConditions.Mentor = &LearningSupport{Available: false, Evidence: e}
	if !o.MatchesSupport("self") {
		t.Fatal("explicit absence not distinguished from unknown")
	}
	for _, r := range []*LearningRange{{Min: 12, Max: 8, Unit: "hours_per_week", Evidence: e}, {Min: 0, Max: 12, Unit: "hours_per_week", Evidence: e}, {Min: 10, Max: 200, Unit: "hours_per_week", Evidence: e}, {Min: 20, Max: 20, Unit: "hours", Evidence: e}} {
		if r.Valid(true) {
			t.Fatal("invalid weekly workload accepted", r)
		}
	}
}
func TestLearningEvidenceAndDurationValidation(t *testing.T) {
	e := learningTestEvidence()
	for _, unit := range []string{"month", "week", "day", "year"} {
		if !(&LearningRange{Min: 1, Max: 3, Unit: unit, Evidence: e}).Valid(false) {
			t.Fatal(unit)
		}
	}
	for _, bad := range []LearningEvidence{{Source: "http://school.example/course", Locator: e.Locator, Digest: e.Digest, CheckedAt: e.CheckedAt}, {Source: e.Source, Locator: "", Digest: e.Digest, CheckedAt: e.CheckedAt}, {Source: e.Source, Locator: e.Locator, Digest: "not-a-digest", CheckedAt: e.CheckedAt}, {Source: e.Source, Locator: e.Locator, Digest: e.Digest, CheckedAt: e.CheckedAt.Add(time.Hour)}} {
		if bad.Valid() {
			t.Fatal("invalid evidence accepted", bad)
		}
	}
}

func TestLearningSupportRequiresExplicitAvailability(t *testing.T) {
	evidence, _ := json.Marshal(learningTestEvidence())
	for _, raw := range []string{`{"evidence":` + string(evidence) + `}`, `{"available":null,"evidence":` + string(evidence) + `}`} {
		var fact LearningSupport
		if json.Unmarshal([]byte(raw), &fact) == nil {
			t.Fatal("missing support fact became confirmed absence")
		}
	}
	var fact LearningSupport
	if err := json.Unmarshal([]byte(`{"available":false,"evidence":`+string(evidence)+`}`), &fact); err != nil || fact.Available || !fact.Evidence.Valid() {
		t.Fatal(fact, err)
	}
}
