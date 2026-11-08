package catalog

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAISubjectUsesTitleOrCurriculumNotRecommendationsOrSupport(t *testing.T) {
	for _, title := range []string{"ИИ в Python-проектах", "Spring AI: LLM-приложения на Java", "Machine Learning: подготовка к собеседованию", "Конструирование нейронных сетей", "LangChain / LangGraph и RAG", "Scikit-Learn: машинное обучение", "Без опыта: нейросети на Python", "Компьютерное зрение с PyTorch", "ChatGPT для разработчиков"} {
		if !IsAICourse(Course{Title: title}) {
			t.Fatal("missed explicit AI subject", title)
		}
	}
	for _, title := range []string{"HTML и CSS", "MAIL server", "Программирование без ИИ", "Python без ChatGPT и нейросетей", "Основы Python"} {
		c := Course{Title: title, Summary: "AI Review проверяет задания; после основ можно перейти к машинному обучению.", Recommendations: &CourseRecommendations{Directions: &RecommendationTags{Values: []string{"data"}, Quote: "AI-generated advice about content"}}}
		if IsAICourse(c) {
			t.Fatal("marketing, substrings or AI advice became an AI subject", title)
		}
	}
	if !IsAICourse(Course{Title: "Прикладной Python", Topics: []string{"Векторный поиск и LLM API"}}) {
		t.Fatal("explicit curriculum ignored")
	}
}

func discoveryFixtures(now time.Time) []Course {
	providers := []string{"Яндекс Практикум", "OTUS", "Хекслет", "Stepik", "Skillbox", "Нетология", "Skillfactory", "Skypro", "HTML Academy", "PurpleSchool", "CodeBasics", "JavaRush", "JavaScript.ru"}
	courses := []Course{}
	for pi, provider := range providers {
		for n := 0; n < 3; n++ {
			c := fixture(now)
			c.ID = fmt.Sprintf("course-%02d-%d", pi, n)
			c.Provider = provider
			c.Title = "Обычный курс"
			c.Language = "go"
			c.Direction = "backend"
			c.Goals = []string{"job"}
			c.Audience = []string{"basics"}
			c.CheckedAt = now.Add(-time.Duration(pi*10+n) * time.Hour)
			for j := range c.Offers {
				c.Offers[j].ID = c.ID + "-" + c.Offers[j].ID
			}
			courses = append(courses, c)
		}
		if pi == 1 || pi == 3 || pi == 4 {
			for n := 0; n < 6; n++ {
				c := fixture(now)
				c.ID = fmt.Sprintf("ai-%02d-%d", pi, n)
				c.Provider = provider
				c.Title = "LLM и нейросети"
				c.Language = "python"
				c.Direction = "backend"
				c.Goals = []string{"job"}
				c.Audience = []string{"basics"}
				c.CheckedAt = now.Add(-time.Duration(n) * time.Hour)
				for j := range c.Offers {
					c.Offers[j].ID = c.ID + "-" + c.Offers[j].ID
				}
				courses = append(courses, c)
			}
		}
	}
	return courses
}

func TestDiscoveryDiverseSchoolsEveryThirdAIAndStablePagination(t *testing.T) {
	now := time.Now().UTC()
	courses := discoveryFixtures(now)
	f := Filter{Sort: "discovery", Page: 1, PageSize: 18}
	got := Search(courses, f, now)
	if len(got) != len(courses) {
		t.Fatal("lost courses", len(got))
	}
	providers := map[string]bool{}
	ids := map[string]bool{}
	for i, item := range got {
		if ids[item.Course.ID] {
			t.Fatal("duplicate", item.Course.ID)
		}
		ids[item.Course.ID] = true
		if i < 18 {
			providers[schoolKey(item.Course)] = true
			if item.AICourse != ((i+1)%3 == 0) {
				t.Fatalf("wrong AI slot %d: %s", i+1, item.Course.ID)
			}
		}
	}
	if len(providers) != 13 {
		t.Fatal("not all schools represented", providers)
	}
	if got[0].Course.Provider != "Яндекс Практикум" || got[1].Course.Provider != "OTUS" || got[3].Course.Provider != "Хекслет" || got[2].Course.Provider != "Stepik" {
		t.Fatal("priority schools missing", got[:4])
	}
	// The complete order cannot depend on the requested page or on input order.
	f.Page = 2
	f.PageSize = 12
	if !reflect.DeepEqual(got, Search(courses, f, now)) {
		t.Fatal("page-dependent ranking")
	}
	for i, j := 0, len(courses)-1; i < j; i, j = i+1, j-1 {
		courses[i], courses[j] = courses[j], courses[i]
	}
	if !reflect.DeepEqual(got, Search(courses, f, now)) {
		t.Fatal("unstable tie or map order")
	}
}

func TestDiscoveryShortageAndHardFiltersNeverInsertUnmatchedCourses(t *testing.T) {
	now := time.Now().UTC()
	courses := discoveryFixtures(now)
	for _, f := range []Filter{{Language: "go"}, {Language: "python"}, {Language: "go", Support: "review", Hours: ptr(8)}, {Language: "python", Max: ptr(int64(1999999))}, {Goal: "try"}} {
		strict := Search(courses, f, now)
		f.Sort = "discovery"
		got := Search(courses, f, now)
		want := map[string]string{}
		for _, v := range strict {
			want[v.Course.ID] = v.Offer.ID
		}
		if len(got) != len(strict) {
			t.Fatal("filter mismatch", f)
		}
		for _, v := range got {
			if want[v.Course.ID] != v.Offer.ID {
				t.Fatal("course or tariff replaced", v)
			}
		}
		if f.Language == "go" {
			for _, v := range got {
				if v.AICourse {
					t.Fatal("AI quota bypassed language filter")
				}
			}
		}
	}
	for _, courses := range [][]Course{nil, courses[:1], courses[3:5]} {
		if len(Search(courses, Filter{Sort: "discovery"}, now)) != len(courses) {
			t.Fatal("short list lost or duplicated courses")
		}
	}
}

func TestDiscoveryPreservesConfirmationGroupsEvenWhenOnlyAdviceIsAI(t *testing.T) {
	now := time.Now().UTC()
	courses := discoveryFixtures(now)[:7]
	for n := 0; n < 8; n++ {
		c := fixture(now)
		c.ID = fmt.Sprintf("advice-%d", n)
		c.Title = "Нейросети"
		c.Goals = []string{"unknown"}
		c.Source = "https://school.example/course"
		c.Profile = nil
		if n < 4 {
			c.Recommendations = &CourseRecommendations{Goals: &RecommendationTags{Values: []string{"job"}, Quote: "Программа включает практические проекты и машинное обучение", Evidence: LearningEvidence{Source: c.Source, Locator: "program", Digest: strings.Repeat("a", 64), CheckedAt: now}}}
		}
		courses = append(courses, c)
	}
	got := Search(courses, Filter{Sort: "discovery", Goal: "job", IncludeRecommendations: true, IncludeUnknownProfiles: true}, now)
	if len(got) != 15 {
		t.Fatal(len(got))
	}
	for i, item := range got {
		want := "confirmed"
		if i >= 7 {
			want = "recommended"
		}
		if i >= 11 {
			want = "unverified"
		}
		if item.ProfileMatch != want {
			t.Fatal("quota crossed confidence boundary", i, item.ProfileMatch)
		}
	}
}

func TestSchoolAliasesAndSourceIdentity(t *testing.T) {
	if schoolKey(Course{Provider: "Code Basics (Хекслет)"}) != schoolKey(Course{Provider: "CodeBasics"}) {
		t.Fatal("alias split")
	}
	if schoolKey(Course{Provider: "OTUS", Source: "https://stepik.org/course/1/promo"}) != "stepik" {
		t.Fatal("provider name overrode actual platform")
	}
	if schoolKey(Course{Provider: "School", Source: "https://ru.hexlet.io/courses/python"}) != "hexlet" {
		t.Fatal("subdomain lost")
	}
}
