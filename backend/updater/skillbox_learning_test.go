package updater

import (
	"devcourse-finder/catalog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSkillboxLearningKeepsCourseAndTariffsSeparate(t *testing.T) {
	data, e := os.ReadFile("testdata/learning/skillbox.html")
	if e != nil {
		t.Fatal(e)
	}
	c := Candidate{Adapter: "skillbox", URL: "https://skillbox.ru/course/profession-python", ExternalID: "profession-python"}
	now := time.Now().UTC()
	r := catalog.Course{ID: "skillbox-profession-python", Offers: []catalog.Offer{{ID: "skillbox-profession-python-t76"}, {ID: "skillbox-profession-python-t1777"}, {ID: "skillbox-profession-python-t1776"}}}
	r, o := skillboxLearning(data, learningDocument(data), r, Observation{}, c, now)
	for _, offer := range r.Offers {
		if offer.LearningConditions == nil || offer.LearningConditions.Duration == nil || offer.LearningConditions.Duration.Max != 10 {
			t.Fatal(offer)
		}
	}
	if !validLearningEnvelope(QueuedObservation{Candidate: c, Record: r, Observation: o, ObservedAt: now}) {
		t.Fatal("invalid envelope")
	}
	changed := []byte(strings.ReplaceAll(string(data), "program-v6__stat", "payment-stat"))
	r, _ = skillboxLearning(changed, learningDocument(changed), r, Observation{}, c, now)
	for _, offer := range r.Offers {
		if offer.LearningConditions != nil && offer.LearningConditions.Duration != nil {
			t.Fatal("billing used as course duration")
		}
	}
}
