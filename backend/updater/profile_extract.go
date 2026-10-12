package updater

import (
	"devcourse-finder/catalog"
	"strings"
	"time"
)

func profileText(raw string) string {
	return strings.ToLower(strings.Join(strings.Fields(nodeText(learningDocument([]byte(raw)))), " "))
}
func profileHas(text string, phrases ...string) bool {
	for _, p := range phrases {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}
func profileTags(text string, proof catalog.LearningEvidence, kind string) *catalog.ProfileTags {
	text = profileText(text)
	// Negated or conditional descriptions require a more specific source contract.
	if profileHas(text, "не подходит", "не для", "только после", "если успешно", "возможно", "например", "рекомендуемые курсы") {
		return nil
	}
	values := []string{}
	add := func(value string, phrases ...string) {
		if profileHas(text, phrases...) {
			values = append(values, value)
		}
	}
	if kind == "audience" {
		add("none", "без опыта", "без знаний программирования", "начинающих", "новичков", "новичкам", "никогда не программировал", "никогда не сталкивался с javascript", "не знаком с программированием", "с нуля")
		add("basics", "знает основы", "знаете основы", "знаком с основами", "с базовыми знаниями", "уже знакомы с основами")
		add("projects", "пишет свои проекты", "пишете свои проекты", "опыт создания проектов", "собственные проекты")
		add("working", "работающих разработчиков", "работающим разработчикам", "опытных разработчиков", "практикующих разработчиков", "программистов с опытом", "опытным программистам")
		add("switch", "с другого языка", "других языках", "на другом языке", "освоить новый язык")
	} else {
		add("try", "попробовать программирование", "познакомиться с программированием", "первые шаги в программировании")
		add("job", "первой работе", "первую работу", "устроиться на работу", "стать разработчиком", "стать python-разработчиком", "подготовиться к трудоустройству")
		add("switch", "сменить направление", "сменить язык", "перейти с другого языка")
		add("deepen", "углубить знания", "углубите знания", "повысить квалификацию", "расширить знания")
	}
	if len(values) == 0 {
		return nil
	}
	return &catalog.ProfileTags{Values: values, Evidence: proof}
}
func profileRequirement(raw, language string, proof catalog.LearningEvidence) *catalog.ProfileRequirement {
	text := profileText(raw)
	if profileHas(text, "желательно", "рекомендуется", "необязательно", "не обязательны", "возможно", "например", "не требуется коммерческий", "не нужен коммерческий", "не требуется опыт коммерческой", "не требуется опыт работы", "без коммерческого опыта") {
		return nil
	}
	levels := []string{}
	if profileHas(text, "не нужен опыт", "не нужны знания", "не требуется опыт", "не требуются знания", "не требуется знание", "без опыта", "с нуля", "никаких специальных знаний") {
		levels = append(levels, "none")
	}
	if profileHas(text, "знать основы", "знаете основы", "необходимы базовые знания", "нужны базовые знания", "требуются базовые знания", "знание основ", "sufficient base knowledge") {
		levels = append(levels, "basics")
	}
	if profileHas(text, "опыт создания проектов", "уметь создавать проекты", "опыт разработки проектов") {
		levels = append(levels, "projects")
	}
	if profileHas(text, "коммерческий опыт", "опыт коммерческой разработки", "опыт работы разработчиком") {
		levels = append(levels, "working")
	}
	if len(levels) != 1 {
		return nil
	}
	scope := "programming"
	if language != "" && strings.Contains(text, language) && !profileHas(text, "любого языка", "одного из языков", "других языках") {
		scope = "course_language"
	}
	return &catalog.ProfileRequirement{Level: levels[0], Scope: scope, Evidence: proof}
}
func profileDirection(raw string, proof catalog.LearningEvidence) *catalog.ProfileValue {
	text := profileText(raw)
	values := []string{}
	add := func(value string, phrases ...string) {
		if profileHas(text, phrases...) {
			values = append(values, value)
		}
	}
	if profileHas(text, "например", "рекомендуемые", "другие курсы") {
		return nil
	}
	add("basics", "курс посвящен основам", "курс посвящён основам", "изучите основы языка", "основы программирования", "базовые конструкции языка")
	add("backend", "бэкенд-разработк", "backend-разработк", "серверной разработк", "разрабатывать серверные приложения")
	add("frontend", "фронтенд-разработк", "frontend-разработк", "клиентской разработк")
	add("fullstack", "fullstack-разработк", "фулстек-разработк")
	add("automation", "автоматизации тестирования", "автоматизировать тестирование")
	if len(values) != 1 {
		return nil
	}
	return &catalog.ProfileValue{Value: values[0], Evidence: proof}
}
func profileAttach(r catalog.Course, o Observation, p *catalog.CourseProfile) (catalog.Course, Observation) {
	if len(p.Evidence()) == 0 {
		p = nil
	}
	r.Profile = p
	o.ProfileObserved = true
	o.Profile = p
	return r, o
}
func stepikProfile(data []byte, course stepikCourse, c Candidate, now time.Time) *catalog.CourseProfile {
	source := candidateEndpoint(c)
	proof := func(field string) catalog.LearningEvidence {
		return learningProof(data, source, "courses[id="+c.ExternalID+"]."+field, now)
	}
	p := &catalog.CourseProfile{}
	language := courseLanguage(course.Title, course.Summary)
	p.Prerequisites = profileRequirement(course.Requirements, language, proof("requirements"))
	p.Audience = profileTags(course.TargetAudience, proof("target_audience"), "audience")
	p.ExcludedAudience = profileExcludedAudience(course.TargetAudience, proof("target_audience"))
	// Summary is the current course's declared subject, not a title classifier.
	p.Direction = profileDirection(course.Summary, proof("summary"))
	stepikDescriptionProfile(data, course, c, now, p)
	return p
}

// Preserve explicit negative suitability separately; unknown opt-in must never
// reinterpret 'not for beginners' as an unverified beginner recommendation.
func profileExcludedAudience(raw string, proof catalog.LearningEvidence) *catalog.ProfileTags {
	text := profileText(raw)
	values := []string{}
	for _, rule := range []struct {
		value   string
		phrases []string
	}{
		{"none", []string{"не для новичков", "не подходит новичкам", "не подходит начинающим", "не для начинающих", "not for beginners"}},
		{"basics", []string{"не для знающих основы", "не подходит людям с базовыми знаниями"}},
		{"working", []string{"не для опытных разработчиков", "не подходит работающим разработчикам"}},
	} {
		if profileHas(text, rule.phrases...) {
			values = append(values, rule.value)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return &catalog.ProfileTags{Values: values, Evidence: proof}
}
func profileOutcomeLevel(raw string, proof catalog.LearningEvidence) *catalog.ProfileValue {
	text := profileText(raw)
	if profileHas(text, "например", "не гарантируется", "возможно") {
		return nil
	}
	levels := []string{}
	for _, rule := range []struct{ value, phrase string }{
		{"basics", "целевой уровень: базовые знания"}, {"projects", "целевой уровень: самостоятельные проекты"}, {"working", "целевой уровень: коммерческая разработка"},
	} {
		if strings.Contains(text, rule.phrase) {
			levels = append(levels, rule.value)
		}
	}
	if len(levels) != 1 {
		return nil
	}
	return &catalog.ProfileValue{Value: levels[0], Evidence: proof}
}
