package updater

import (
	"devcourse-finder/catalog"
	"testing"
	"time"
)

func TestLearningTextDoesNotConvertBillingOrVideoHours(t *testing.T) {
	proof := learningProof([]byte("source"), "https://practicum.yandex.ru/backend-developer", "bound-card", time.Now().UTC())
	for _, s := range []string{"В рассрочку на 36 месяцев", "Доступ на 12 месяцев", "149 часов теории", "Не нужно 10 часов в неделю", "До 6 месяцев", "2 занятия по 2 ак.ч. в неделю"} {
		if learningTextRange(s, false, proof) != nil || learningTextRange(s, true, proof) != nil {
			t.Fatal("invented range", s)
		}
	}
	for _, s := range []string{"2–4 месяца", "Курс проходится от 2 до 4 месяцев в зависимости от интенсивности"} {
		v := learningTextRange(s, false, proof)
		if v == nil || v.Min != 2 || v.Max != 4 || v.Unit != "month" {
			t.Fatal(s, v)
		}
	}
	v := learningTextRange("Нужно выделять от 6 до 12 часов в неделю", true, proof)
	if v == nil || v.Min != 6 || v.Max != 12 {
		t.Fatal(v)
	}
}
func TestPracticumLearningUsesOnlyTargetSKU(t *testing.T) {
	now := time.Now().UTC()
	c := Candidate{Adapter: "yandex", ExternalID: "backend-developer", URL: "https://practicum.yandex.ru/backend-developer"}
	page := []byte(`<div class="common-flow-card__wrapper" id="python-developer-plus"><p>30 часов в неделю</p><p>Индивидуальные консультации с наставником</p></div><footer>10 месяцев обучения</footer>`)
	p := practicumProfession{}
	v := practicumLearning(page, []byte(`{}`), c, c.ExternalID, p, now)
	if len(v.Evidence()) != 0 {
		t.Fatal("other tariff leaked", v)
	}
	page = []byte(`<div class="common-flow-card__wrapper" id="backend-developer"><p>От 10 до 20 часов в неделю</p><p>Проекты проверим вручную</p></div>`)
	v = practicumLearning(page, []byte(`{}`), c, c.ExternalID, p, now)
	if v.WeeklyWorkload == nil || v.Review == nil || !v.Review.Available || v.Mentor != nil {
		t.Fatal(v)
	}
	known := false
	offer := catalog.Offer{SupportKnown: &known}
	applyLearningConditions(&offer, v)
	applyLearningConditions(&offer, nil)
	if offer.LearningConditions != nil || offer.Hours != nil || offer.Review {
		t.Fatal("vanished facts retained", offer)
	}
}
