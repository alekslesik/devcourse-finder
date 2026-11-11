package updater

import (
	"devcourse-finder/catalog"
	"reflect"
	"testing"
	"time"
)

func TestProfileAttachmentKeepsRequirementsInsteadOfFrameworkBeginnerGuess(t *testing.T) {
	c := Candidate{Adapter: "purpleschool", URL: "https://purpleschool.ru/course/nextjs"}
	proof := learningProof([]byte("official course"), c.URL, "course.requirements", time.Now().UTC())
	profile := &catalog.CourseProfile{
		Prerequisites: &catalog.ProfileRequirement{Level: "basics", Scope: "course_language", Evidence: proof},
		Audience:      &catalog.ProfileTags{Values: []string{"none"}, Evidence: proof},
		Direction:     &catalog.ProfileValue{Value: "frontend", Evidence: proof},
	}
	r, o := profileAttach(catalog.Course{}, Observation{}, profile)
	if !r.Profile.Valid() || !o.ProfileObserved || r.Profile.Audience != nil || r.Profile.Prerequisites == nil || r.Profile.Direction == nil {
		t.Fatal("one ambiguous phrase poisoned unrelated facts", r.Profile, o)
	}
	if catalog.MatchProfile(r, "experience", "none") != "incompatible" || catalog.MatchProfile(r, "direction", "frontend") != "confirmed" {
		t.Fatal("requirements or supported direction lost")
	}
}

func TestProfileAttachmentAbstainsOnConflictingAudienceAndTarget(t *testing.T) {
	proof := learningProof([]byte("official course"), "https://otus.ru/lessons/python", "course.program", time.Now().UTC())
	p := &catalog.CourseProfile{
		Prerequisites:    &catalog.ProfileRequirement{Level: "basics", Scope: "programming", Evidence: proof},
		TargetLevel:      &catalog.ProfileValue{Value: "none", Evidence: proof},
		Audience:         &catalog.ProfileTags{Values: []string{"basics"}, Evidence: proof},
		ExcludedAudience: &catalog.ProfileTags{Values: []string{"basics"}, Evidence: proof},
	}
	r, o := profileAttach(catalog.Course{}, Observation{}, p)
	if !o.ProfileObserved || !r.Profile.Valid() || r.Profile.TargetLevel != nil || r.Profile.Audience != nil || r.Profile.ExcludedAudience == nil {
		t.Fatal(r.Profile, o)
	}
	p.Prerequisites.Evidence.Digest = "invalid"
	r, o = profileAttach(catalog.Course{}, Observation{}, p)
	if o.ProfileObserved || o.Profile != nil || r.Profile != nil {
		t.Fatal("invalid extraction became an authoritative disappearance")
	}
}

func TestNativeProfileConflictPreservesCuratedProfileAndOtherFields(t *testing.T) {
	proof := learningProof([]byte("official course"), "https://otus.ru/lessons/python", "course.audience", time.Now().UTC())
	course := catalog.Course{ID: "curated", Audience: []string{"none"}, Direction: "backend", Profile: &catalog.CourseProfile{
		Audience: &catalog.ProfileTags{Values: []string{"none"}, Evidence: proof},
	}}
	before := course
	applyCourseProfile(&course, &catalog.CourseProfile{Prerequisites: &catalog.ProfileRequirement{Level: "basics", Scope: "programming", Evidence: proof}})
	if !reflect.DeepEqual(course, before) || !course.Profile.Valid() {
		t.Fatal("incoming conflict damaged curated facts", course)
	}
}
