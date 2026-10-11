package updater

import (
	"testing"
	"time"
)

func TestPurpleLearningSeparatesAIReviewChatAndPersonalMentor(t *testing.T) {
	data, c := purpleFixture(t, "javascript-basics")
	now := time.Now().UTC()
	r, o, e := collectPurpleSchool(data, c, now)
	if e != nil {
		t.Fatal(e)
	}
	if r.Offers[1].LearningConditions != nil && r.Offers[1].LearningConditions.Review != nil {
		t.Fatal("AI became human review")
	}
	v := r.Offers[2].LearningConditions
	if v == nil || v.Review == nil || !v.Review.Available || v.Community == nil || v.Mentor != nil || r.Offers[2].Mentor {
		t.Fatal(v)
	}
	if !validLearningEnvelope(QueuedObservation{Candidate: c, Record: r, Observation: o, ObservedAt: now}) {
		t.Fatal("invalid evidence")
	}
}
