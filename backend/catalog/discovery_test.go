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

func TestDiscoverySpreadsFiniteSchoolsAndAIOverEveryCatalogPage(t *testing.T) {
	now := time.Now().UTC()
	// The live catalog is heavily skewed toward one platform. A plain round-robin
	// consumes every smaller school within five pages and AI within eleven.
	stock := []struct {
		provider  string
		total, ai int
	}{
		{"Яндекс Практикум", 12, 0}, {"OTUS", 14, 1}, {"Хекслет", 4, 0},
		{"Stepik", 722, 61}, {"Skillbox", 4, 3}, {"Нетология", 2, 0},
		{"Skillfactory", 1, 0}, {"Skypro", 1, 0}, {"HTML Academy", 3, 0},
		{"PurpleSchool", 14, 0}, {"CodeBasics", 5, 0}, {"JavaRush", 1, 0},
		{"JavaScript.ru", 1, 0},
	}
	var courses []Course
	for pi, school := range stock {
		for n := 0; n < school.total; n++ {
			c := fixture(now)
			c.ID = fmt.Sprintf("stock-%02d-%03d", pi, n)
			c.Provider = school.provider
			c.Title = "Программирование"
			if n < school.ai {
				c.Title = "LLM-приложения"
			}
			courses = append(courses, c)
		}
	}
	got := Search(courses, Filter{Sort: "discovery"}, now)
	if len(got) != 784 {
		t.Fatal("missing courses", len(got))
	}
	ids := map[string]bool{}
	aiTotal := 0
	for start, page := 0, 1; start < len(got); start, page = start+18, page+1 {
		end := start + 18
		if end > len(got) {
			end = len(got)
		}
		schools, ai := map[string]bool{}, 0
		for _, item := range got[start:end] {
			if ids[item.Course.ID] {
				t.Fatal("duplicate across pages", item.Course.ID)
			}
			ids[item.Course.ID] = true
			schools[schoolKey(item.Course)] = true
			if item.AICourse {
				ai++
			}
		}
		aiTotal += ai
		if len(schools) < 2 || ai < 1 {
			t.Fatalf("page %d exhausted finite inventory: schools=%v AI=%d", page, schools, ai)
		}
		if page == 1 && (len(schools) != 13 || ai != 6) {
			t.Fatalf("initial overview narrowed: schools=%v AI=%d", schools, ai)
		}
	}
	if aiTotal != 65 {
		t.Fatal("AI subjects invented or lost", aiTotal)
	}
	// Page-size changes only regroup the same global order, including far pages.
	if !reflect.DeepEqual(got, Search(courses, Filter{Sort: "discovery", Page: 40, PageSize: 12}, now)) {
		t.Fatal("far-page request reranked the catalog")
	}
}

func TestDiscoveryHandlesIntersectingSchoolAndSubjectShortages(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name                          string
		ordinary, ai                  [2]int
		schoolsEveryPage, aiEveryPage bool
	}{
		{"AI only at smaller school", [2]int{144, 0}, [2]int{0, 18}, true, true},
		{"AI and ordinary at smaller school", [2]int{144, 12}, [2]int{0, 18}, true, true},
		{"AI only at dominant school", [2]int{0, 18}, [2]int{144, 0}, true, true},
		{"all AI", [2]int{}, [2]int{90, 12}, true, true},
		{"sparse AI does not block ordinary smaller schools", [2]int{144, 18}, [2]int{0, 3}, true, false},
		{"one school", [2]int{70, 0}, [2]int{1, 0}, false, false},
		{"one AI course", [2]int{90, 18}, [2]int{1, 0}, false, false},
		{"one smaller-school course", [2]int{90, 1}, [2]int{30, 0}, false, false},
		{"sparse stock reaches the last page", [2]int{180, 5}, [2]int{6, 0}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var courses []Course
			for pi, provider := range []string{"Stepik", "Independent author"} {
				for n := 0; n < tc.ordinary[pi]+tc.ai[pi]; n++ {
					c := fixture(now)
					c.ID = fmt.Sprintf("%d-%03d", pi, n)
					c.Provider = provider
					c.Title = "Программирование"
					if n < tc.ai[pi] {
						c.Title = "Нейросети"
					}
					courses = append(courses, c)
				}
			}
			before := Search(courses, Filter{Sort: "recent"}, now)
			got := Search(courses, Filter{Sort: "discovery"}, now)
			want := map[string]string{}
			for _, item := range before {
				want[item.Course.ID] = item.Offer.ID
			}
			for _, item := range got {
				if want[item.Course.ID] != item.Offer.ID {
					t.Fatal("course duplicated, invented or tariff replaced", item.Course.ID)
				}
				delete(want, item.Course.ID)
			}
			if len(want) != 0 {
				t.Fatal("valid courses dropped", len(want))
			}
			if tc.name == "sparse stock reaches the last page" {
				last := got[len(got)/18*18:]
				smallerSchool, ai := false, false
				for _, item := range last {
					smallerSchool = smallerSchool || item.Course.Provider == "Independent author"
					ai = ai || item.AICourse
				}
				if !smallerSchool || !ai {
					t.Fatal("sparse stock was consumed before the tail")
				}
			}
			if !tc.schoolsEveryPage {
				return
			}
			for start := 0; start < len(got); start += 18 {
				end := start + 18
				if end > len(got) {
					end = len(got)
				}
				schools, ai := map[string]bool{}, 0
				for _, item := range got[start:end] {
					schools[schoolKey(item.Course)] = true
					if item.AICourse {
						ai++
					}
				}
				if len(schools) < 2 || (tc.aiEveryPage && ai == 0) {
					t.Fatalf("page %d lost diversity despite sufficient stock: %v, AI=%d", start/18+1, schools, ai)
				}
			}
		})
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
