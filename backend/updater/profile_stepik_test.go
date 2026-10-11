package updater

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestStepikProfileUsesDedicatedFieldsNotCourseTitle(t *testing.T) {
	raw, err := os.ReadFile("testdata/profiles/stepik-go.json")
	if err != nil {
		t.Fatal(err)
	}
	var course stepikCourse
	if json.Unmarshal(raw, &course) != nil {
		t.Fatal("fixture")
	}
	c := Candidate{Adapter: "stepik", ExternalID: "54403", URL: "https://stepik.org/course/54403/promo"}
	p := stepikProfile(raw, course, c, time.Now().UTC())
	if p.Audience != nil || p.Prerequisites != nil || p.Goals != nil {
		t.Fatal("broad audience and optional recommendation became exact", p)
	}
	course.TargetAudience = "Курс для начинающих без опыта и разработчиков на других языках"
	course.Requirements = "Не требуется опыт программирования"
	p = stepikProfile(raw, course, c, time.Now().UTC())
	if p.Audience == nil || len(p.Audience.Values) != 2 || p.Prerequisites == nil || p.Prerequisites.Level != "none" {
		t.Fatal(p)
	}
	course.TargetAudience = "Курс не для начинающих. Рекомендуемые курсы для опытных разработчиков"
	course.Title = "Python для начинающих с нуля"
	if p = stepikProfile(raw, course, c, time.Now().UTC()); p.Audience != nil {
		t.Fatal("negation/title leaked", p)
	}
}
