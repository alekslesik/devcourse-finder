package updater

import (
	"os"
	"testing"
	"time"
)

func TestJavaRushLearningDoesNotInheritOtherSubscriptionSupport(t *testing.T) {
	data, e := os.ReadFile("testdata/javarush-prices.html")
	if e != nil {
		t.Fatal(e)
	}
	prices, e := os.ReadFile("testdata/javarush-monthly.json")
	if e != nil {
		t.Fatal(e)
	}
	c := Candidate{Adapter: "javarush", ExternalID: "java-premium", URL: "https://javarush.com/prices"}
	now := time.Now().UTC()
	r, o, e := collectJavaRush(data, prices, c, now)
	if e != nil {
		t.Fatal(e)
	}
	v := r.Offers[0].LearningConditions
	if v != nil && len(v.Evidence()) != 0 {
		t.Fatal("automatic checker, help section or Mentor Pro became Premium facts", v)
	}
	if !o.LearningObserved || !validLearningEnvelope(QueuedObservation{Candidate: c, Record: r, Observation: o, ObservedAt: now}) {
		t.Fatal(o)
	}
}
