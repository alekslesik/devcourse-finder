package updater

import (
	"devcourse-finder/catalog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSkillfactoryLearningIgnoresAcademicHoursAndRecommendations(t *testing.T) {
	data, e := os.ReadFile("testdata/learning/skillfactory.html")
	if e != nil {
		t.Fatal(e)
	}
	c := Candidate{Adapter: "skillfactory", URL: "https://skillfactory.ru/python-developer"}
	now := time.Now().UTC()
	r := catalog.Course{Offers: []catalog.Offer{{ID: "basic"}, {ID: "personal"}, {ID: "personal-plus"}}}
	r, o := skillfactoryLearning(data, learningDocument(data), r, Observation{}, c, now)
	for _, offer := range r.Offers {
		v := offer.LearningConditions
		if v == nil || v.Duration == nil || v.Duration.Max != 10 || v.WeeklyWorkload != nil || v.Mentor != nil {
			t.Fatal(v)
		}
	}
	if !validLearningEnvelope(QueuedObservation{Candidate: c, Record: r, Observation: o, ObservedAt: now}) {
		t.Fatal("invalid envelope")
	}
	changed := []byte(strings.ReplaceAll(string(data), "10 месяцев", "600 академических часов") + `<section>14 месяцев Индивидуальные консультации с наставником</section>`)
	r, _ = skillfactoryLearning(changed, learningDocument(changed), r, Observation{}, c, now)
	for _, offer := range r.Offers {
		if offer.LearningConditions != nil {
			t.Fatal("invented conditions", offer)
		}
	}
}
