package updater

import (
	"testing"
	"time"
)

func TestCodeBasicsLearningUsesOnlyVerifiedLanding(t *testing.T) {
	c := Candidate{Adapter: "codebasics", URL: "https://code-basics.com/ru/languages/python"}
	now := time.Now().UTC()
	p := basicsPage{}
	p.Props.Landing.Duration = "27 часов"
	p.Props.Landing.Description = "Помощь AI без ограничений. Автоматическая проверка решений."
	v := basicsLearning([]byte(`source`), p, c, now)
	if v.TotalStudyHours == nil || v.TotalStudyHours.Max != 27 || v.TotalStudyHours.Unit != "hour" || v.Duration != nil || v.WeeklyWorkload != nil || v.Review != nil || v.Mentor != nil || v.Community != nil {
		t.Fatal("total study hours lost or became calendar/workload/support", v)
	}
	// Contract mutation: a future explicit calendar duration remains literal.
	p.Props.Landing.Duration = "2–4 недели"
	p.Props.Landing.Description = "Учебный чат. Ручная проверка домашних заданий."
	v = basicsLearning([]byte(`source`), p, c, now)
	if v.Duration == nil || v.Duration.Unit != "week" || v.Duration.Max != 4 || v.Review == nil || v.Community == nil || v.Mentor != nil || v.WeeklyWorkload != nil {
		t.Fatal(v)
	}
}
