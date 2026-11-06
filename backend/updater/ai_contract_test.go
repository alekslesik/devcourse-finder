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

func TestAIWeeklyWorkloadRequiresExplicitBoundUnitsEvenAfterModelApproval(t *testing.T) {
	for _, tc := range []struct {
		quote  string
		lo, hi float64
		valid  bool
	}{
		{"Нагрузка: 1-3. Программа: основы Python для начинающих.", 1, 3, false},
		{"Нагрузка: 12–15 часов. Программа изучения языка Go.", 12, 15, false},
		{"В программе всего 120 часов видеолекций и заданий.", 120, 120, false},
		{"На обучение требуется 8–12 часов в неделю.", 8, 12, true},
		{"Workload: 1.5–3 hours per week for the Python course.", 1.5, 3, true},
		{"Обучение 1–3 месяца. Нагрузка: 8–12 часов в неделю.", 1, 3, false},
	} {
		t.Run(tc.quote, func(t *testing.T) {
			c, docs, x, _ := aiFixture()
			for i := range docs {
				docs[i].Text = tc.quote
				docs[i].Digest = fingerprint([]byte(tc.quote))
			}
			x.Claims = []AIClaim{{Field: "weekly_workload", DocumentID: "offer", OfferID: "stepik-42-course", Quote: tc.quote, Min: valueFloat(tc.lo), Max: valueFloat(tc.hi), Unit: "hours_per_week"}}
			_, err := aiCompile(c, docs, x, AIVerification{Complete: true, Verified: []int{0}}, time.Now())
			if (err == nil) != tc.valid {
				t.Fatal("weekly unit grounding", err)
			}
		})
	}
}

func TestAIBareWorkloadIsDiscardedWithoutLosingAudience(t *testing.T) {
	c, docs, x, _ := aiFixture()
	text := "Курс разработан для начинающих без опыта программирования. Нагрузка: 1-3."
	for i := range docs {
		docs[i].Text = text
		docs[i].Digest = fingerprint([]byte(text))
	}
	x.Claims[1].Quote = "Нагрузка: 1-3."
	x.Claims[1].Min = valueFloat(1)
	x.Claims[1].Max = valueFloat(3)
	isolated, err := aiIsolateClaims(c, docs, x, time.Now())
	if err != nil || isolated.DiscardedClaims != 1 || len(isolated.Claims) != 1 || isolated.Claims[0].Field != "audience" {
		t.Fatal(isolated, err)
	}
}
