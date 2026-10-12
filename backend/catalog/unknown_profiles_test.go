package catalog

import (
	"net/url"
	"testing"
	"time"
)

func TestUnknownProfilesAreOptInAndNeverOverrideIncompatibility(t *testing.T) {
	now := time.Now()
	known := fixture(now)
	known.ID = "confirmed"
	known.Audience = []string{"basics"}
	known.Goals = []string{"job"}
	known.Direction = "backend"
	unknown := known
	unknown.ID = "unknown"
	unknown.Audience = []string{"unknown"}
	unknown.CheckedAt = now.Add(time.Hour)
	incompatible := known
	incompatible.ID = "incompatible"
	incompatible.Audience = []string{"working"}
	strict := Filter{Experience: "basics", Goal: "job", Direction: "backend", Sort: "recent"}
	courses := []Course{unknown, incompatible, known}
	if rows := Search(courses, strict, now); len(rows) != 1 || rows[0].Course.ID != "confirmed" {
		t.Fatal(rows)
	}
	strict.IncludeUnknownProfiles = true
	rows := Search(courses, strict, now)
	if len(rows) != 2 || rows[0].Course.ID != "confirmed" || rows[1].ProfileMatch != "unverified" || len(rows[1].UnknownProfileFields) != 1 || rows[1].UnknownProfileFields[0] != "experience" {
		t.Fatal(rows)
	}
	for _, reason := range rows[1].Reasons {
		if reason == "Подходит вашему опыту" {
			t.Fatal("unverified suitability promised")
		}
	}
	// An unknown audience cannot bypass a verified prerequisite.
	unknown.Profile = &CourseProfile{Prerequisites: &ProfileRequirement{Level: "working", Scope: "programming"}}
	if len(Search([]Course{unknown}, strict, now)) != 0 {
		t.Fatal("known prerequisite bypassed")
	}
	strict.Max = ptr(int64(0))
	if len(Search(courses, strict, now)) != 0 {
		t.Fatal("price constraint relaxed")
	}
	if _, err := Parse(url.Values{"include_unknown_profiles": {"perhaps"}}); err == nil {
		t.Fatal("invalid boolean accepted")
	}
}
