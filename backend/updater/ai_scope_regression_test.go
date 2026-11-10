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

func TestAICourseCurriculumIncludesTaughtContentButExcludesOtherProducts(t *testing.T) {
	for _, tc := range []struct{ provider, program, project string }{
		{"otus", "Программа", "Проектная работа"},
		{"htmlacademy", "Программа курса", "Учебный проект"},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			raw := []byte(`<nav><h2>` + tc.program + `</h2><p>MENU_ONLY: список других программ обучения.</p></nav><section><h2>` + tc.program + `</h2><h3>Разработка приложений</h3><p>CURRICULUM_ONLY: работа с типами данных, обработка ошибок, тестирование и разработка приложений.</p></section><section><h2>` + tc.project + `</h2><p>PROJECT_ONLY: самостоятельный проект с API, хранением данных и автоматическими тестами.</p></section><aside class="related-course"><h2>` + tc.program + `</h2><p>OTHER_PRODUCT: программа соседнего курса.</p></aside><section class="tariffs"><h2>` + tc.program + `</h2><p>VIP_ONLY: персональное наставничество в дорогом тарифе.</p></section><form><h2>` + tc.project + `</h2><p>FORM_ONLY: оставьте контактные данные для консультации.</p></form>`)
			text := aiCourseSections(raw, Candidate{Adapter: tc.provider})
			for _, want := range []string{"CURRICULUM_ONLY", "PROJECT_ONLY"} {
				if !strings.Contains(text, want) {
					t.Fatal("course content omitted", want, text)
				}
			}
			for _, bad := range []string{"MENU_ONLY", "OTHER_PRODUCT", "VIP_ONLY", "FORM_ONLY"} {
				if strings.Contains(text, bad) {
					t.Fatal("out-of-scope text admitted", bad, text)
				}
			}
		})
	}
}

func TestAIPurpleCurriculumDoesNotBorrowAITariffProjects(t *testing.T) {
	raw := []byte(`<section><h2>План курса</h2><h3>Функции и коллекции</h3><p>COMMON_PROGRAM: основы языка, функции, коллекции и работа с файловой системой.</p></section><section><h2>Что будем писать?</h2><p>AI и тренажёры</p><p>OPTIONAL_AI_PROJECT: CRM система и менеджер паролей для дополнительного тарифа.</p></section>`)
	text := aiCourseSections(raw, Candidate{Adapter: "purpleschool"})
	if !strings.Contains(text, "COMMON_PROGRAM") || strings.Contains(text, "OPTIONAL_AI_PROJECT") {
		t.Fatal("optional project gallery became shared curriculum", text)
	}
}

func TestAIAmbiguousCurriculumDoesNotInventAScope(t *testing.T) {
	raw := []byte(`<section><h2>Программа</h2><p>FIRST_PROGRAM: подробная программа курса с практическими заданиями на Python.</p></section><section><h2>Программа</h2><p>SECOND_PROGRAM: другая подробная программа с практическими заданиями на Java.</p></section><section><h2>Проектная работа</h2><p>BOUND_PROJECT: текущий проект курса с разработкой приложения и автоматическими тестами.</p></section>`)
	text := aiCourseSections(raw, Candidate{Adapter: "otus"})
	if strings.Contains(text, "FIRST_PROGRAM") || strings.Contains(text, "SECOND_PROGRAM") || !strings.Contains(text, "BOUND_PROJECT") {
		t.Fatal("ambiguous program leaked or independent project lost", text)
	}
}
