package updater

import (
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func learningFixture(c Candidate, now time.Time) *catalog.LearningConditions {
	e := catalog.LearningEvidence{Source: c.URL, Locator: "course.conditions", Digest: strings.Repeat("a", 64), CheckedAt: now}
	return &catalog.LearningConditions{Duration: &catalog.LearningRange{Min: 3, Max: 5, Unit: "month", Evidence: e}, WeeklyWorkload: &catalog.LearningRange{Min: 8, Max: 12, Unit: "hours_per_week", Evidence: e}, Review: &catalog.LearningSupport{Available: true, Evidence: e}, Community: &catalog.LearningSupport{Available: true, Evidence: e}}
}
func TestLearningOwnershipPreservesCuratedAndClearsMissingCollectedFacts(t *testing.T) {
	c := Candidate{URL: "https://otus.ru/lessons/python"}
	facts := learningFixture(c, time.Now().UTC())
	unknown := false
	o := catalog.Offer{SupportKnown: &unknown}
	applyLearningConditions(&o, facts)
	if o.LearningOwnership != learningDuration|learningWorkload|learningReview|learningCommunity || o.Hours != nil || o.DurationMonths != nil || !o.Review || *o.SupportKnown {
		t.Fatal("range became exact or partial support became complete", o)
	}
	// An independently curated mentor field survives an unknown observation.
	curated := &catalog.LearningSupport{Available: true, Evidence: facts.Review.Evidence}
	o.LearningConditions.Mentor = curated
	applyLearningConditions(&o, nil)
	if o.LearningConditions == nil || o.LearningConditions.Mentor != curated || o.LearningConditions.Duration != nil || o.LearningConditions.WeeklyWorkload != nil || o.LearningConditions.Review != nil || o.Review || o.LearningOwnership != 0 {
		t.Fatal("vanished facts retained or curated field erased", o)
	}
	// Older imported integer fields remain authoritative until explicitly migrated.
	weeks, hours := 10, 6
	known := true
	legacy := catalog.Offer{Weeks: &weeks, Hours: &hours, SupportKnown: &known, Review: true}
	applyLearningConditions(&legacy, facts)
	if legacy.Weeks == nil || *legacy.Weeks != 10 || *legacy.Hours != 6 || !legacy.Review || legacy.LearningOwnership&learningDuration != 0 || legacy.LearningOwnership&learningWorkload != 0 || legacy.LearningOwnership&learningReview != 0 {
		t.Fatal("curated legacy fields overwritten", legacy)
	}
}
func TestLearningEnvelopeRejectsOtherCourseOfferAndConfirmationTime(t *testing.T) {
	c := Candidate{Adapter: "otus", ExternalID: "python", URL: "https://otus.ru/lessons/python"}
	now := time.Now().UTC()
	facts := learningFixture(c, now)
	e := QueuedObservation{Candidate: c, ObservedAt: now, Record: catalog.Course{Offers: []catalog.Offer{{ID: "a", LearningConditions: facts}}}, Observation: Observation{LearningByOffer: map[string]*catalog.LearningConditions{"a": facts}}}
	if !validLearningEnvelope(e) {
		t.Fatal("valid envelope")
	}
	e.Observation.LearningByOffer = map[string]*catalog.LearningConditions{"other": facts}
	if validLearningEnvelope(e) {
		t.Fatal("other tariff accepted")
	}
	e.Observation.LearningByOffer = map[string]*catalog.LearningConditions{"a": facts}
	facts.Review.Evidence.Source = "https://otus.ru/lessons/other"
	if validLearningEnvelope(e) {
		t.Fatal("other course accepted")
	}
	facts.Review.Evidence.Source = c.URL
	facts.Review.Evidence.CheckedAt = now.Add(-time.Hour)
	if validLearningEnvelope(e) {
		t.Fatal("old evidence presented as freshly observed")
	}
}
