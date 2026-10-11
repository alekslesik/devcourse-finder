package updater

import (
	"os"
	"testing"
	"time"
)

func TestRSSchoolLearningCommunityDoesNotPromiseLaterStageMentoring(t *testing.T) {
	data, e := os.ReadFile("testdata/learning/rsschool.html")
	if e != nil {
		t.Fatal(e)
	}
	c := Candidate{Adapter: "rsschool", URL: "https://rs.school/courses/javascript"}
	now := time.Now().UTC()
	v := rsLearning(data, learningDocument(data), c, now)
	if v.Community == nil || !v.Community.Available || v.Duration != nil || v.Mentor != nil || v.Review != nil {
		t.Fatal(v)
	}
	data = []byte(`<section><h2>Learning path</h2><p>Stage 2 lasts 13 weeks. After qualification you will be assigned a personal mentor.</p></section><footer>Here is link for the course Discord server</footer>`)
	if len(rsLearning(data, learningDocument(data), c, now).Evidence()) != 0 {
		t.Fatal("conditional/global support published")
	}
}
