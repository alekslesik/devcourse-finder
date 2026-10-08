package updater

import (
	"bytes"
	"devcourse-finder/catalog"
	"encoding/json"
	"testing"
	"time"
)

func TestRecordHasUnknownClassificationsAndSafeText(t *testing.T) {
	zero := int64(0)
	c, err := normalizedRecord(Candidate{Adapter: "otus", ExternalID: "python-basic", URL: "https://otus.ru/lessons/python-basic"}, "<b>Python</b>", "<p>Программа курса</p><script>danger()</script><style>hidden</style>", "OTUS", Observation{Price: &zero, Enrollment: "open"}, time.Now())
	if err != nil || c.Summary != "Программа курса" || c.Audience[0] != "unknown" || c.Goals[0] != "unknown" {
		t.Fatalf("misleading/unsafe record %#v %v", c, err)
	}
	raw, _ := json.Marshal(catalog.Dataset{Domains: []string{"otus.ru"}, Courses: []catalog.Course{c}})
	if _, err = catalog.Decode(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	for _, f := range []catalog.Filter{{Experience: "experienced"}, {Experience: "none"}, {Goal: "job"}, {Support: "self"}} {
		if len(catalog.Search([]catalog.Course{c}, f, time.Now())) != 0 {
			t.Fatal("unknown evidence matched a confirmed filter", f)
		}
	}
	if len(catalog.Search([]catalog.Course{c}, catalog.Filter{}, time.Now())) != 1 {
		t.Fatal("unknown classification hid valid course")
	}
}
func TestIncompleteOrMixedLanguageRecordsAreNotPublished(t *testing.T) {
	for _, title := range []string{"General programming", "Python and Java", "<script>Python</script>"} {
		zero := int64(0)
		if _, err := normalizedRecord(Candidate{Adapter: "otus", ExternalID: "course", URL: "https://otus.ru/lessons/course"}, title, "Learning", "OTUS", Observation{Price: &zero, Enrollment: "open"}, time.Now()); err == nil {
			t.Fatal("ambiguous record accepted", title)
		}
	}
}
func TestKnownPaidUnknownPriceDoesNotMatchBudgetOrFree(t *testing.T) {
	c, err := normalizedRecord(Candidate{Adapter: "otus", ExternalID: "python-basic", URL: "https://otus.ru/lessons/python-basic"}, "Python", "Курс Python", "OTUS", Observation{PriceUnknown: true, Enrollment: "open"}, time.Now())
	if err != nil || c.Offers[0].Free || c.Offers[0].Price != nil {
		t.Fatal("unknown price classified free", err)
	}
	max := int64(5000000)
	if len(catalog.Search([]catalog.Course{c}, catalog.Filter{Budget: "free"}, time.Now())) != 0 || len(catalog.Search([]catalog.Course{c}, catalog.Filter{Max: &max}, time.Now())) != 0 {
		t.Fatal("unknown price matches confirmed price")
	}
}
