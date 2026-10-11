package updater

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

// Extraction helpers do not select a source scope. Each adapter supplies a
// course/tariff-bound block after its independent identity/availability checks.
func learningProof(data []byte, source, locator string, now time.Time) catalog.LearningEvidence {
	return catalog.LearningEvidence{Source: source, Locator: locator, Digest: fingerprint(data), CheckedAt: now}
}
func learningDocument(data []byte) *html.Node { d, _ := html.Parse(bytes.NewReader(data)); return d }
func classToken(n *html.Node, token string) bool {
	for _, s := range strings.Fields(nodeAttr(n, "class")) {
		if s == token {
			return true
		}
	}
	return false
}
func learningNodes(root *html.Node, match func(*html.Node) bool) []*html.Node {
	var out []*html.Node
	if root == nil {
		return out
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "nav" || n.Data == "footer" || n.Data == "template" || n.Data == "noscript") {
			return
		}
		if match(n) {
			out = append(out, n)
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return out
}
func learningOne(root *html.Node, match func(*html.Node) bool) *html.Node {
	a := learningNodes(root, match)
	if len(a) == 1 {
		return a[0]
	}
	return nil
}

var learningDurationText = regexp.MustCompile(`^(?:курс проходится |обучение |длительность(?: обучения| курса)?[: ]+)?(?:от )?([0-9]+(?:[.,][0-9]+)?)(?:\s*(?:–|—|-|до)\s*([0-9]+(?:[.,][0-9]+)?))?\s*(месяц(?:а|ев)?|недел(?:я|и|ь)|дн(?:я|ей)|год(?:а|ов)?)(?: обучения| в зависимости от интенсивности)?[.!]?$`)
var learningWeeklyText = regexp.MustCompile(`(?:^|[^0-9])(?:от )?([0-9]+(?:[.,][0-9]+)?)(?:\s*(?:–|—|-|до)\s*([0-9]+(?:[.,][0-9]+)?))?\s*час(?:а|ов)?\s*в\s*неделю(?:\b|[.! ,]|$)`)

func learningTextRange(text string, weekly bool, evidence catalog.LearningEvidence) *catalog.LearningRange {
	text = strings.ToLower(strings.Join(strings.Fields(text), " "))
	// Negated claims, access/subscription/billing and video runtime are never facts.
	for _, s := range []string{"рассроч", "доступ", "подписк", "кредит", "видеоурок", "не потребуется", "не нужно", "не более", "до ", "от " + "0", "~", "≈", "примерно"} {
		// 'от X до Y' is a genuine range, 'до X' alone is not an exact duration.
		if s == "до " {
			if strings.HasPrefix(text, s) {
				return nil
			}
			continue
		}
		if strings.Contains(text, s) {
			return nil
		}
	}
	re := learningDurationText
	if weekly {
		re = learningWeeklyText
	}
	matches := re.FindAllStringSubmatch(text, -1)
	if len(matches) != 1 {
		return nil
	}
	m := matches[0]
	lo, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
	hi := lo
	if m[2] != "" {
		hi, _ = strconv.ParseFloat(strings.ReplaceAll(m[2], ",", "."), 64)
	}
	unit := "hours_per_week"
	if !weekly {
		switch {
		case strings.HasPrefix(m[3], "месяц"):
			unit = "month"
		case strings.HasPrefix(m[3], "недел"):
			unit = "week"
		case strings.HasPrefix(m[3], "дн"):
			unit = "day"
		default:
			unit = "year"
		}
	}
	v := &catalog.LearningRange{Min: lo, Max: hi, Unit: unit, Evidence: evidence}
	c := &catalog.LearningConditions{Duration: v}
	if weekly {
		c = &catalog.LearningConditions{WeeklyWorkload: v}
	}
	if !c.Valid() {
		return nil
	}
	return v
}

// Conflicting values in the same authoritative block are unknown, not first-win.
func learningUniqueRange(nodes []*html.Node, weekly bool, proof catalog.LearningEvidence) *catalog.LearningRange {
	var result *catalog.LearningRange
	for _, n := range nodes {
		v := learningTextRange(nodeText(n), weekly, proof)
		if v == nil {
			continue
		}
		if result != nil && (result.Min != v.Min || result.Max != v.Max || result.Unit != v.Unit) {
			return nil
		}
		result = v
	}
	return result
}
func learningSupportText(text string, evidence catalog.LearningEvidence) *catalog.LearningConditions {
	text = strings.ToLower(strings.Join(strings.Fields(text), " "))
	c := &catalog.LearningConditions{}
	// Only explicit descriptions; AI checks and general mentor titles are omitted.
	field := func(positive, negative []string) *catalog.LearningSupport {
		yes, no := false, false
		for _, s := range positive {
			if strings.Contains(text, s) {
				yes = true
			}
		}
		for _, s := range negative {
			if strings.Contains(text, s) {
				no = true
			}
		}
		if no {
			return &catalog.LearningSupport{Available: false, Evidence: evidence}
		}
		if yes {
			return &catalog.LearningSupport{Available: true, Evidence: evidence}
		}
		return nil
	}
	c.Review = field([]string{"ручная проверка домашних заданий", "проверка домашних заданий экспертом", "ревью от опытных наставников", "проекты проверим вручную", "наставник будет проверять качество выполнения заданий", "наставник проверяет задания", "проверка заданий наставником", "код-ревью от наставника", "персональное код-ревью"}, []string{"без проверки заданий", "без проверки домашних заданий", "без код-ревью"})
	c.Mentor = field([]string{"индивидуальные консультации с наставником", "персональных консультаций с наставником", "индивидуальные консультации ментора", "индивидуальные консультации с ментором", "личные встречи с наставником"}, []string{"без индивидуальных консультаций", "без личных встреч с наставником"})
	if c.Mentor == nil && regexp.MustCompile(`индивидуальн(?:ые|ых) (?:встреч(?:и)?|консультаци(?:и|й)) с (?:опытным [a-zа-я-]+-разработчиком|экспертом)(?:[ .,:]|$)`).MatchString(text) {
		c.Mentor = &catalog.LearningSupport{Available: true, Evidence: evidence}
	}
	c.Community = field([]string{"учебном чате", "учебный чат", "чат с менторами", "чат с наставником", "чат с наставниками", "сообщество студентов", "course discord server"}, []string{"без учебного чата"})
	return c
}
func learningAttach(r catalog.Course, o Observation, byOffer map[string]*catalog.LearningConditions) (catalog.Course, Observation) {
	o.LearningObserved = false
	o.LearningConditions = nil
	o.LearningByOffer = map[string]*catalog.LearningConditions{}
	for i := range r.Offers {
		id := r.Offers[i].ID
		v := byOffer[id]
		if v != nil && len(v.Evidence()) == 0 {
			v = nil
		}
		r.Offers[i].LearningConditions = v
		o.LearningByOffer[id] = v
	}
	return r, o
}
func learningLocator(prefix string, id int64) string { return fmt.Sprintf("%s[id=%d]", prefix, id) }
