package catalog

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Detect a subject, not AI-generated advice or an AI support feature. Summaries
// often mention ML as a future application of basic Python, so they are excluded.
var aiSubject = regexp.MustCompile(`(?i)(нейросет|нейронн\p{L}*\s+сет|искусственн\p{L}*\s+интеллект|машинн\p{L}*\s+обучен|глубок\p{L}*\s+обучен|компьютерн\p{L}*\s+зрени|machine\s+learning|deep\s+learning|computer\s+vision|chatgpt|deepseek|claude|langchain|langgraph|tensorflow|pytorch|scikit[ -]?learn|stable\s+diffusion|midjourney|(^|[^\p{L}\p{N}])(ai|ии|ml|llm|nlp|gpt|rag)([^\p{L}\p{N}]|$))`)
var withoutAI = regexp.MustCompile(`(?i)(^|[^\p{L}\p{N}])без\s+(ии|ai|нейросет|нейронн|chatgpt|gpt|claude|deepseek)([^\p{L}\p{N}]|\p{L}*($|[^\p{L}\p{N}]))`)

func IsAICourse(c Course) bool {
	if withoutAI.MatchString(c.Title) {
		return false
	}
	if aiSubject.MatchString(c.Title) {
		return true
	}
	for _, topic := range c.Topics {
		if !withoutAI.MatchString(topic) && aiSubject.MatchString(topic) {
			return true
		}
	}
	return false
}

var discoverySchools = []string{"practicum", "otus", "hexlet", "stepik", "skillbox", "netology", "skillfactory", "skypro", "htmlacademy", "purpleschool", "codebasics", "javarush", "javascript"}

func schoolKey(c Course) string {
	u, _ := url.Parse(c.Source)
	host := ""
	if u != nil {
		host = strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	}
	for _, v := range [][2]string{{"practicum.yandex.ru", "practicum"}, {"practicum.yandex.com", "practicum"}, {"otus.ru", "otus"}, {"hexlet.io", "hexlet"}, {"stepik.org", "stepik"}, {"skillbox.ru", "skillbox"}, {"netology.ru", "netology"}, {"skillfactory.ru", "skillfactory"}, {"sky.pro", "skypro"}, {"skypro.ru", "skypro"}, {"htmlacademy.ru", "htmlacademy"}, {"purpleschool.ru", "purpleschool"}, {"code-basics.com", "codebasics"}, {"javarush.com", "javarush"}, {"javarush.ru", "javarush"}, {"javascript.ru", "javascript"}} {
		if host == v[0] || strings.HasSuffix(host, "."+v[0]) {
			return v[1]
		}
	}
	name := strings.Join(strings.Fields(strings.ToLower(c.Provider)), " ")
	for _, v := range [][2]string{{"яндекс практикум", "practicum"}, {"практикум", "practicum"}, {"хекслет", "hexlet"}, {"code basics (хекслет)", "codebasics"}, {"code basics", "codebasics"}, {"html academy", "htmlacademy"}, {"javascript.ru", "javascript"}} {
		if name == v[0] {
			return v[1]
		}
	}
	return name
}

func schoolPriority(key string) int {
	for i, name := range discoverySchools {
		if name == key {
			return i
		}
	}
	return len(discoverySchools)
}

type discoverySchool struct {
	key   string
	items [2][]Result // Ordinary courses and explicitly AI-related subjects.
	next  [2]int
	shown int
}

// Apply once to the complete matching set, before HTTP pagination. No course is
// added, removed or duplicated, and a later page is a continuation of this order.
// Confirmation groups remain strict boundaries; an AI-content quota never
// promotes an unverified match ahead of a confirmed result.
func discoveryOrder(in []Result) []Result {
	out := make([]Result, 0, len(in))
	shown := map[string]int{}
	for start := 0; start < len(in); {
		end := start + 1
		for end < len(in) && in[end].ProfileMatch == in[start].ProfileMatch {
			end++
		}
		bySchool := map[string]*discoverySchool{}
		for _, item := range in[start:end] {
			key := schoolKey(item.Course)
			if bySchool[key] == nil {
				bySchool[key] = &discoverySchool{key: key, shown: shown[key]}
			}
			lane := 0
			if item.AICourse {
				lane = 1
			}
			p := bySchool[key]
			p.items[lane] = append(p.items[lane], item)
		}
		pools := make([]*discoverySchool, 0, len(bySchool))
		for _, pool := range bySchool {
			pools = append(pools, pool)
		}
		sort.Slice(pools, func(i, j int) bool {
			a, b := pools[i].key, pools[j].key
			if schoolPriority(a) != schoolPriority(b) {
				return schoolPriority(a) < schoolPriority(b)
			}
			return a < b
		})
		for len(out) < end {
			lane := 0
			if (len(out)+1)%3 == 0 {
				lane = 1
			}
			var best *discoverySchool
			for attempt := 0; attempt < 2 && best == nil; attempt++ {
				for _, pool := range pools {
					if pool.next[lane] < len(pool.items[lane]) && (best == nil || pool.shown < best.shown) {
						best = pool
					}
				}
				if best == nil {
					lane = 1 - lane
				}
			}
			out = append(out, best.items[lane][best.next[lane]])
			best.next[lane]++
			best.shown++
			shown[best.key] = best.shown
		}
		start = end
	}
	return out
}
