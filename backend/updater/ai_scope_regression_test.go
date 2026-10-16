package updater

import (
	"devcourse-finder/catalog"
	"os"
	"strings"
	"testing"
)

func TestAIExpansionCourseTextExcludesPurchaseAndPrivateBlocks(t *testing.T) {
	c := Candidate{Adapter: "skillbox"}
	raw := []byte(`<section><h2>Кому подойдёт курс</h2><p>COURSE_ONLY: длительное описание начинающим разработчикам без опыта программирования.</p></section><li data-tariff-data='{}'><h2>Чему вы научитесь</h2><p>VIP_ONLY: персональные занятия для вип тарифа</p></li><div class="payment-plan-card"><h2>Что нужно знать перед началом</h2><p>PRIVATE_PURCHASE: условия другого продукта</p></div><script>SESSION_SECRET</script><form><h2>Кому подойдёт этот курс</h2><p>CONTACT_EMAIL</p></form><aside class="related-course"><h2>Кому подойдёт курс</h2><p>OTHER_COURSE</p></aside>`)
	text := aiCourseSections(raw, c)
	if !strings.Contains(text, "COURSE_ONLY") {
		t.Fatal(text)
	}
	for _, secret := range []string{"VIP_ONLY", "PRIVATE_PURCHASE", "SESSION_SECRET", "CONTACT_EMAIL", "OTHER_COURSE"} {
		if strings.Contains(text, secret) {
			t.Fatal("leaked", secret)
		}
	}
}
func TestAISkillboxRealTierFeaturesStaySeparate(t *testing.T) {
	raw, e := os.ReadFile("testdata/learning/skillbox.html")
	if e != nil {
		t.Fatal(e)
	}
	c := Candidate{Adapter: "skillbox"}
	r := catalog.Course{ID: "skillbox-test"}
	_, offers, e := aiSkillboxDocuments(raw, c, r)
	if e != nil {
		t.Fatal(e)
	}
	basic := offers[r.ID+"-t76"]
	if !strings.Contains(basic, "10 месяцев") || strings.Contains(basic, "индивидуальных консультаций") {
		t.Fatal("base lost duration or gained VIP support", basic)
	}
	if len(offers) < 2 {
		t.Fatal("fixture tiers missing")
	}
}
func TestAIRegistryOnlyEnablesSupportedAcademyFormat(t *testing.T) {
	if _, ok := aiCandidateURL("https://htmlacademy.ru/profession/frontend"); ok {
		t.Fatal("profession enabled by intensive contract")
	}
}
