package updater

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestAcademyLearningPreservesRangeAndPersonalSupport(t *testing.T) {
	data, e := os.ReadFile("testdata/learning/htmlacademy.html")
	if e != nil {
		t.Fatal(e)
	}
	c := Candidate{Adapter: "htmlacademy", URL: "https://htmlacademy.ru/intensive/react"}
	now := time.Now().UTC()
	v := academyLearning(data, c, now)
	if v == nil || v.Duration == nil || v.Duration.Min != 2 || v.Duration.Max != 4 || v.Mentor == nil || !v.Mentor.Available || v.WeeklyWorkload != nil {
		t.Fatal(v)
	}
	changed := []byte(strings.ReplaceAll(string(data), "Курс проходится от 2 до 4 месяцев в зависимости от интенсивности", "Доступ на 12 месяцев"))
	v = academyLearning(changed, c, now)
	if v.Duration != nil {
		t.Fatal("access duration", v)
	}
	if academyLearning([]byte(`<div class="intensive-weeks-section"><h3>Групповой формат</h3><li>2 месяца</li></div>`), c, now) != nil {
		t.Fatal("wrong format")
	}
}
