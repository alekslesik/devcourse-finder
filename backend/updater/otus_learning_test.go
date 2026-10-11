package updater

import (
	"os"
	"testing"
	"time"
)

func TestOtusLearningCalendarAndStatedWeeklyRange(t *testing.T) {
	data, e := os.ReadFile("testdata/learning/otus.html")
	if e != nil {
		t.Fatal(e)
	}
	c := Candidate{Adapter: "otus", URL: "https://otus.ru/lessons/python-basic"}
	now := time.Now().UTC()
	node := map[string]any{"hasCourseInstance": map[string]any{"@type": "CourseInstance", "duration": "P6M"}}
	v := otusLearning(data, node, c, now)
	if v.Duration == nil || v.Duration.Max != 6 || v.WeeklyWorkload == nil || v.WeeklyWorkload.Min != 6 || v.WeeklyWorkload.Max != 12 {
		t.Fatal(v)
	}
	node["hasCourseInstance"] = map[string]any{"@type": "CourseInstance", "duration": "PT120H"}
	v = otusLearning([]byte(`<footer>20 часов в неделю</footer>`), node, c, now)
	if len(v.Evidence()) != 0 {
		t.Fatal("video/footer facts", v)
	}
}
