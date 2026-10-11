package updater

import (
	"testing"
)

func TestNetologyLearningRenderedTargetCard(t *testing.T) {
	data, c := netologyFixture(t, "go")
	now := netologyNow()
	r, o, e := collectNetology(data, c, now)
	if e != nil {
		t.Fatal(e)
	}
	if !o.LearningObserved || r.Offers[0].LearningConditions == nil || r.Offers[0].LearningConditions.Duration == nil {
		t.Fatal("missing rendered duration", o.LearningConditions)
	}
	if r.Offers[0].LearningConditions.Duration.Max != 9 {
		t.Fatal("other tariff duration", r.Offers[0].LearningConditions)
	}
	if !validLearningEnvelope(QueuedObservation{Candidate: c, Record: r, Observation: o, ObservedAt: now}) {
		t.Fatal("invalid evidence")
	}
}
