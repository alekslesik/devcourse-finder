package updater

import (
	"encoding/json"
	"testing"
	"time"
)

func TestStepikLearningExplicitWorkloadNotTotalsOrEnrollment(t *testing.T) {
	c := Candidate{Adapter: "stepik", ExternalID: "58852", URL: "https://stepik.org/course/58852/promo"}
	now := time.Now().UTC()
	course := stepikCourse{Workload: "От 3 до 5 часов в неделю", Description: `<section><h2>Поддержка на курсе</h2><p>Ручная проверка домашних заданий. Учебный чат.</p></section><section><h2>Другие курсы</h2><p>Индивидуальные консультации с наставником</p></section>`}
	raw, _ := json.Marshal(course)
	v := stepikLearning(raw, course, c, now)
	if v.WeeklyWorkload == nil || v.WeeklyWorkload.Max != 5 || v.Review == nil || v.Community == nil || v.Mentor != nil {
		t.Fatal(v)
	}
	for _, workload := range []string{"120 часов видеолекций", "2 часа в день", "12 месяцев доступа", ""} {
		course.Workload = workload
		course.Description = `<footer>Учебный чат</footer>`
		if len(stepikLearning(raw, course, c, now).Evidence()) != 0 {
			t.Fatal(workload)
		}
	}
}
