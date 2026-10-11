package updater

import (
	"os"
	"strings"
	"testing"
	"time"
)

func TestSkyproLearningHeroDoesNotUseUnrelatedPeriods(t *testing.T) {
	data, e := os.ReadFile("testdata/learning/skypro.html")
	if e != nil {
		t.Fatal(e)
	}
	c := Candidate{Adapter: "skypro", URL: "https://sky.pro/course/programming/python-web-course"}
	now := time.Now().UTC()
	doc := learningDocument(data)
	v := skyproLearning(data, doc, learningDocument([]byte(`<div>Стоимость</div>`)), c, now)
	if v.Duration == nil || v.Duration.Max != 12 || v.Mentor != nil {
		t.Fatal(v)
	}
	changed := []byte(strings.ReplaceAll(string(data), "12 месяцев", "Доступ на 12 месяцев") + `<div class="t-rec"><div class="tn-atom">4 месяца</div></div>`)
	v = skyproLearning(changed, learningDocument(changed), learningDocument([]byte(`<div>Стоимость</div>`)), c, now)
	if v.Duration != nil {
		t.Fatal("unrelated/access period", v)
	}
}
