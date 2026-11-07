package updater

import (
	"devcourse-finder/catalog"
	"testing"
	"time"
)

func TestTotalStudyHoursLiteralFieldNotRuntimeOrRate(t *testing.T) {
	e := learningProof([]byte("source"), "https://stepik.org/api/courses/42", "courses[id=42].workload", time.Now().UTC())
	for _, tc := range []struct {
		text   string
		lo, hi float64
	}{
		{"27 часов", 27, 27}, {"Всего: 10–20 часов обучения", 10, 20}, {"От 10 до 20 часов на прохождение курса", 10, 20}, {"1,5 часа", 1.5, 1.5},
	} {
		h := learningTotalHoursRange(tc.text, e)
		if h == nil || h.Min != tc.lo || h.Max != tc.hi || h.Unit != "hour" {
			t.Fatalf("%s: %+v", tc.text, h)
		}
	}
	for _, text := range []string{"от 20 часов", "до 20 часов", "примерно 20 часов", "20+ часов", "20 часов и более", "20 академических часов", "120 часов видеолекций", "2 часа в день", "3–5 часов в неделю", "12 месяцев доступа", "~20 часов", "20 часов, включая 5 часов видео", "0 часов", "20–10 часов", "10001 час", ""} {
		if h := learningTotalHoursRange(text, e); h != nil {
			t.Fatalf("ambiguous effort %q: %+v", text, h)
		}
	}
	c := Candidate{Adapter: "stepik", ExternalID: "42", URL: "https://stepik.org/course/42/promo"}
	facts := stepikLearning([]byte("source"), stepikCourse{Workload: "27 часов"}, c, time.Now().UTC())
	if facts.TotalStudyHours == nil || facts.Duration != nil || facts.WeeklyWorkload != nil {
		t.Fatal(facts)
	}
}

func TestTotalStudyHoursOwnershipAndAIAbstention(t *testing.T) {
	facts := learningFixture(Candidate{URL: "https://school.example/course"}, time.Now().UTC())
	o := catalog.Offer{}
	applyLearningConditions(&o, &catalog.LearningConditions{TotalStudyHours: facts.TotalStudyHours})
	if o.LearningOwnership != learningTotalStudyHours || o.Hours != nil || o.Weeks != nil || o.DurationMonths != nil {
		t.Fatal(o)
	}
	for _, partial := range []bool{false, true} {
		applyAILearning(&o, nil, partial)
		if o.LearningConditions == nil || o.LearningConditions.TotalStudyHours != facts.TotalStudyHours || o.LearningOwnership != learningTotalStudyHours {
			t.Fatal("AI cleared literal total hours", o)
		}
	}
	applyLearningConditions(&o, nil)
	if o.LearningConditions != nil || o.LearningOwnership != 0 {
		t.Fatal("vanished collected hours retained", o)
	}
	o.LearningConditions = &catalog.LearningConditions{TotalStudyHours: facts.TotalStudyHours}
	applyLearningConditions(&o, nil)
	if o.LearningConditions.TotalStudyHours != facts.TotalStudyHours {
		t.Fatal("curated hours erased", o)
	}
}
