package catalog

import (
	"strings"
	"testing"
	"time"
)

func profileProof() LearningEvidence {
	return LearningEvidence{Source: "https://example.com/course", Locator: "course audience", Digest: strings.Repeat("a", 64), CheckedAt: time.Now().UTC()}
}
func TestProfileMatchingSeparatesRequirementsAndAudience(t *testing.T) {
	c := Course{Audience: []string{"unknown"}, Goals: []string{"unknown"}, Direction: "unknown", Profile: &CourseProfile{Prerequisites: &ProfileRequirement{Level: "none", Scope: "programming", Evidence: profileProof()}}}
	if MatchProfile(c, "experience", "working") != "unknown" {
		t.Fatal("minimum implies audience")
	}
	c.Profile.TargetLevel = &ProfileValue{Value: "basics", Evidence: profileProof()}
	if MatchProfile(c, "experience", "basics") != "confirmed" || MatchProfile(c, "experience", "working") != "incompatible" || MatchProfile(c, "experience", "switch") != "unknown" {
		t.Fatal("invalid profile interval")
	}
	c.Profile.Audience = &ProfileTags{Values: []string{"switch"}, Evidence: profileProof()}
	if MatchProfile(c, "experience", "switch") != "confirmed" {
		t.Fatal("switch lost")
	}
	c.Profile.Prerequisites.Level = "projects"
	if MatchProfile(c, "experience", "switch") != "incompatible" {
		t.Fatal("switch silently upgraded to project experience")
	}
	c.Profile.Prerequisites.Level = "none"
	c.Profile.Goals = &ProfileTags{Values: []string{"job"}, Evidence: profileProof()}
	if MatchProfile(c, "goal", "job") != "confirmed" || MatchProfile(c, "goal", "deepen") != "incompatible" {
		t.Fatal("goal semantics")
	}
	c.Profile.Audience.Values = []string{"none", "none"}
	if c.Profile.Valid() {
		t.Fatal("duplicate evidence tag accepted")
	}
}

func TestProfileNegativeAndContradictoryEvidence(t *testing.T) {
	p := &CourseProfile{ExcludedAudience: &ProfileTags{Values: []string{"none"}, Evidence: profileProof()}}
	c := fixture(time.Now())
	c.Audience = []string{"unknown"}
	c.Profile = p
	if MatchProfile(c, "experience", "none") != "incompatible" || !p.Valid() {
		t.Fatal(p)
	}
	if len(Search([]Course{c}, Filter{Experience: "none", IncludeUnknownProfiles: true}, time.Now())) != 0 {
		t.Fatal("explicit exclusion bypassed")
	}
	p.Audience = &ProfileTags{Values: []string{"none"}, Evidence: profileProof()}
	if p.Valid() {
		t.Fatal("contradiction accepted")
	}
	p.ExcludedAudience = nil
	p.Prerequisites = &ProfileRequirement{Level: "basics", Scope: "programming", Evidence: profileProof()}
	if p.Valid() {
		t.Fatal("beginner suitability conflicts with hard prerequisites")
	}
}

func TestVerifiedIntervalIsNotBlockedByPrimaryTypedAudience(t *testing.T) {
	c := fixture(time.Now())
	c.Audience = []string{"unknown"}
	c.Profile = &CourseProfile{Prerequisites: &ProfileRequirement{Level: "none", Scope: "programming", Evidence: profileProof()}, TargetLevel: &ProfileValue{Value: "projects", Evidence: profileProof()}, Audience: &ProfileTags{Values: []string{"none"}, Evidence: profileProof()}}
	if MatchProfile(c, "experience", "basics") != "confirmed" || MatchProfile(c, "experience", "projects") != "confirmed" || MatchProfile(c, "experience", "working") != "incompatible" {
		t.Fatal("verified interval lost")
	}
	c.Profile.TargetLevel = nil
	if MatchProfile(c, "experience", "working") == "confirmed" || MatchProfile(c, "experience", "basics") == "confirmed" {
		t.Fatal("beginner audience blindly expanded")
	}
	// Existing curated legacy classifications remain literal.
	c.Profile.Audience = nil
	c.Profile.TargetLevel = &ProfileValue{Value: "projects", Evidence: profileProof()}
	c.Audience = []string{"none"}
	if MatchProfile(c, "experience", "basics") != "incompatible" {
		t.Fatal("curated legacy tags changed")
	}
}
