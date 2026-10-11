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
