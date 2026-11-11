package updater

import (
	"strings"
	"testing"
)

func TestAINetologyOfferLearningTextExcludesPaymentsAndOtherCards(t *testing.T) {
	raw := []byte(`<section><p name="sections[0].cards[0].title">Go-разработчик</p><p name="sections[0].cards[0].features[0].title">9 месяцев обучения</p><p name="sections[0].cards[0].features[1].title">Проверка домашних работ преподавателем</p><div class="styles_prices__test"><p>36 месяцев рассрочки, 100000 рублей</p></div><p name="sections[0].cards[1].features[0].title">FOREIGN_PLAN: персональный наставник</p><p name="sections[0].cards[0].features[0].other.title">FOREIGN_FIELD</p><button>Купить</button></section>`)
	doc := learningDocument(raw)
	text := aiNetologyOfferLearningText(doc)
	for _, supported := range []string{"9 месяцев обучения", "Проверка домашних работ преподавателем"} {
		if !strings.Contains(text, supported) {
			t.Fatal("learning feature lost", supported, text)
		}
	}
	for _, foreign := range []string{"рассрочки", "100000", "FOREIGN_PLAN", "FOREIGN_FIELD", "Купить"} {
		if strings.Contains(text, foreign) {
			t.Fatal("financial or other-card context leaked", foreign, text)
		}
	}
	ambiguous := learningDocument([]byte(`<section><p name="sections[0].cards[0].title">Go</p><p name="sections[0].cards[1].title">Java</p><p name="sections[0].cards[0].features[0].title">9 месяцев обучения</p></section>`))
	if text := aiNetologyOfferLearningText(ambiguous); text != "" {
		t.Fatal("ambiguous tariff accepted", text)
	}
}
