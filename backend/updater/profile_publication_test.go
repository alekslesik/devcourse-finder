package updater

import (
	"devcourse-finder/catalog"
	"strings"
	"testing"
	"time"
)

func profileFixture(c Candidate, at time.Time) *catalog.CourseProfile {
	return &catalog.CourseProfile{Audience: &catalog.ProfileTags{Values: []string{"none"}, Evidence: catalog.LearningEvidence{Source: c.URL, Locator: "course.audience", Digest: strings.Repeat("a", 64), CheckedAt: at}}}
}
func TestProfileOwnershipIsPerField(t *testing.T) {
	p := profileFixture(Candidate{URL: "https://otus.ru/lessons/python"}, time.Now().UTC())
	c := catalog.Course{Audience: []string{"unknown"}, Goals: []string{"job"}, Direction: "backend"}
	applyCourseProfile(&c, p)
	if c.ProfileOwnership != profileAudience || c.Audience[0] != "none" || c.Goals[0] != "job" || c.Direction != "backend" {
		t.Fatal(c)
	}
	applyCourseProfile(&c, nil)
	if c.Profile != nil || c.ProfileOwnership != 0 || c.Audience[0] != "unknown" {
		t.Fatal(c)
	}
	c.Profile = p
	c.Audience = []string{"none"}
	applyCourseProfile(&c, nil)
	if c.Profile == nil {
		t.Fatal("operator evidence erased")
	}
}
func TestProfileEnvelopeAttestation(t *testing.T) {
	c := Candidate{Adapter: "otus", URL: "https://otus.ru/lessons/python"}
	now := time.Now().UTC()
	p := profileFixture(c, now)
	e := QueuedObservation{Candidate: c, ObservedAt: now, Record: catalog.Course{Profile: p}, Observation: Observation{ProfileObserved: true, Profile: p}}
	if !validProfileEnvelope(e) {
		t.Fatal("valid attestation rejected")
	}
	for _, change := range []func(*QueuedObservation){
		func(e *QueuedObservation) { e.Observation.ProfileObserved = false },
		func(e *QueuedObservation) { e.Record.Profile = nil },
		func(e *QueuedObservation) { e.ObservedAt = now.Add(time.Hour) },
		func(e *QueuedObservation) { e.Candidate.URL = "https://otus.ru/lessons/java" },
	} {
		bad := e
		change(&bad)
		if validProfileEnvelope(bad) {
			t.Fatal("forged evidence accepted")
		}
	}
}
