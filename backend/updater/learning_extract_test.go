package updater

import (
	"devcourse-finder/catalog"
	"os"
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

func TestPracticumCourseReviewRequiresExplicitPlusInheritance(t *testing.T) {
	data, err := os.ReadFile("testdata/learning/practicum.html")
	if err != nil {
		t.Fatal(err)
	}
	c := Candidate{Adapter: "yandex", ExternalID: "backend-developer", URL: "https://practicum.yandex.ru/backend-developer"}
	now := time.Now().UTC()
	for _, slug := range []string{"backend-developer", "python-developer-plus"} {
		v := practicumLearning(data, []byte(`{}`), c, slug, practicumProfession{}, now)
		if v.Review == nil || !v.Review.Available || v.Mentor != nil || v.WeeklyWorkload != nil {
			t.Fatal(slug, v)
		}
	}
	if v := practicumLearning(data, []byte(`{}`), c, "unrelated-plus", practicumProfession{}, now); v.Review != nil {
		t.Fatal("unbound plus inherited base review")
	}
}

func TestLearningSupportNegationAndConflict(t *testing.T) {
	proof := learningProof([]byte("source"), "https://skillbox.ru/course/profession-python", "tariff", time.Now().UTC())
	for _, text := range []string{"Ручная проверка домашних заданий не предусмотрена", "Без проверки домашних заданий"} {
		got := learningSupportText(text, proof)
		if got.Review == nil || got.Review.Available {
			t.Fatal(text, got)
		}
	}
	got := learningSupportText("Без проверки домашних заданий. Ручная проверка домашних заданий", proof)
	if got.Review != nil {
		t.Fatal("conflicting support became certain", got)
	}
	got = learningSupportText("Индивидуальные консультации с экспертом не предусмотрены", proof)
	if got.Mentor != nil && got.Mentor.Available {
		t.Fatal("negated consultation became available", got)
	}
}

func TestLearningRangesRejectOneSidedBounds(t *testing.T) {
	proof := learningProof([]byte("source"), "https://otus.ru/lessons/python-basic", "description", time.Now().UTC())
	for _, text := range []string{"от 6 часов в неделю", "Нагрузка до 6 часов в неделю", "Нужно от 6 часов в неделю", "от 6 месяцев", "Длительность от 6 месяцев", "6 часов в неделю и более"} {
		if learningTextRange(text, false, proof) != nil || learningTextRange(text, true, proof) != nil {
			t.Fatal("one-sided bound became exact", text)
		}
	}
	for _, text := range []string{"от 6 до 12 часов в неделю", "Нагрузка от 6 до 12 часов в неделю", "6–12 часов в неделю"} {
		v := learningTextRange(text, true, proof)
		if v == nil || v.Min != 6 || v.Max != 12 {
			t.Fatal("closed range lost", text, v)
		}
	}
}
