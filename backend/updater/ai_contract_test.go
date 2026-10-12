package updater

import (
	"testing"
	"time"
)

func aiFixture() (Candidate, []AIDocument, AIExtraction, AIVerification) {
	c := Candidate{Adapter: "stepik", ExternalID: "42", URL: "https://stepik.org/course/42/promo"}
	text := "Курс разработан для начинающих без опыта программирования. Обучение требует 8–12 часов в неделю. Задания проверяет преподаватель лично."
	docs := []AIDocument{{ID: "course", Source: candidateEndpoint(c), Scope: "course", Text: text, Digest: fingerprint([]byte(text))}, {ID: "offer", Source: candidateEndpoint(c), Scope: "offer", OfferID: "stepik-42-course", Text: text, Digest: fingerprint([]byte(text))}}
	extraction := AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{{Field: "audience", DocumentID: "course", Quote: "Курс разработан для начинающих без опыта программирования.", Values: []string{"none"}}, {Field: "weekly_workload", DocumentID: "offer", OfferID: "stepik-42-course", Quote: "Обучение требует 8–12 часов в неделю.", Min: valueFloat(8), Max: valueFloat(12), Unit: "hours_per_week"}}}
	return c, docs, extraction, AIVerification{Complete: true, Verified: []int{0, 1}}
}
func valueFloat(n float64) *float64 { return &n }
func TestAIQuotesAndIndependentVerification(t *testing.T) {
	c, docs, x, v := aiFixture()
	at := time.Now().UTC()
	facts, err := aiCompile(c, docs, x, v, at)
	if err != nil || facts.Profile.Audience.Values[0] != "none" || facts.Learning["stepik-42-course"].WeeklyWorkload.Max != 12 {
		t.Fatal(facts, err)
	}
	v.Verified = []int{1}
	facts, err = aiCompile(c, docs, x, v, at)
	if err != nil || facts.Profile != nil {
		t.Fatal("verbatim quote bypassed independent rejection", facts, err)
	}
	for _, mutation := range []func(*AIExtraction){
		func(x *AIExtraction) { x.CourseURL = "https://stepik.org/course/43/promo" },
		func(x *AIExtraction) {
			x.Claims = append([]AIClaim(nil), x.Claims...)
			x.Claims[0].Quote = "Everyone is guaranteed employment and mentors."
		},
		func(x *AIExtraction) {
			x.Claims = append([]AIClaim(nil), x.Claims...)
			x.Claims[1].OfferID = "another-tariff"
		},
		func(x *AIExtraction) {
			x.Claims = append([]AIClaim(nil), x.Claims...)
			x.Claims[1].Max = valueFloat(40)
		},
		func(x *AIExtraction) { x.Complete = false },
		func(x *AIExtraction) { x.Claims = append(x.Claims, x.Claims[0]) },
	} {
		bad := x
		mutation(&bad)
		if _, err = aiCompile(c, docs, bad, v, at); err == nil {
			t.Fatal("unsafe mutation accepted")
		}
	}
}
func TestAIDocumentLimitsAndForeignSource(t *testing.T) {
	c, docs, x, v := aiFixture()
	docs[0].Source = "https://example.com/course"
	if _, err := aiCompile(c, docs, x, v, time.Now()); err == nil {
		t.Fatal("model source accepted")
	}
	if aiStrictJSON([]byte(`{"complete":true,"claims":[],"course_url":"x","price":10}`), &AIExtraction{}) == nil {
		t.Fatal("unknown field accepted")
	}
	if aiStrictJSON([]byte(`{"complete":true,"claims":[],"course_url":"x"} {}`), &AIExtraction{}) == nil {
		t.Fatal("trailing JSON accepted")
	}
}
