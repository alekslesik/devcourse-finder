package updater

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

// These adapters accept complete, explicitly free public programs only. A free
// introductory lesson, an absent price or an advertisement is never sufficient.
func smallFreeObservation(now time.Time) Observation {
	zero := int64(0)
	lease := now.Add(26 * time.Hour)
	return Observation{Price: &zero, Schedule: "flexible", Enrollment: "continuous", ValidUntil: &lease, LearningObserved: true}
}

func collectCodeChick(data []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	var schema map[string]any
	var faq struct {
		Type      string `json:"@type"`
		Questions []struct {
			Name   string `json:"name"`
			Answer struct {
				Text string `json:"text"`
			} `json:"acceptedAnswer"`
		} `json:"mainEntity"`
	}
	doc := learningDocument(data)
	russian, canonicalCount, canonicalOK := false, 0, false
	schemas, faqs := 0, 0
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "html" {
			russian = nodeAttr(n, "lang") == "ru"
		}
		if n.Data == "link" && nodeAttr(n, "rel") == "canonical" {
			canonicalCount++
			canonicalOK = matchingURL(nodeAttr(n, "href"), c.URL)
		}
		if n.Data == "script" && nodeAttr(n, "type") == "application/ld+json" && n.FirstChild != nil {
			if nodeAttr(n, "data-hid") == "ld-course" {
				schemas++
				_ = decodeJSON([]byte(n.FirstChild.Data), &schema)
			}
			if nodeAttr(n, "data-hid") == "ld-course-faq" {
				faqs++
				_ = decodeJSON([]byte(n.FirstChild.Data), &faq)
			}
		}
	})
	bad := func() (catalog.Course, Observation, error) {
		return catalog.Course{}, Observation{}, rejection("unverified_course_offer")
	}
	if c.Adapter != "codechick" || len(data) > maxBody || !russian || canonicalCount != 1 || !canonicalOK || schemas != 1 || faqs != 1 || schema == nil {
		return bad()
	}
	title := schemaString(schema, "name")
	heading := learningOne(doc, func(n *html.Node) bool { return n.Data == "h1" })
	provider, _ := schema["provider"].(map[string]any)
	offer, _ := schema["offers"].(map[string]any)
	instance, _ := schema["hasCourseInstance"].(map[string]any)
	priceJSON, _ := json.Marshal(offer["price"])
	price, err := rubles(priceJSON)
	if heading == nil || nodeText(heading) != title || schema["@type"] != "Course" || !matchingURL(schemaString(schema, "url"), c.URL) || schema["inLanguage"] != "ru" || schema["isAccessibleForFree"] != true || provider["name"] != "CodeChick" || !matchingURL(schemaString(provider, "url"), "https://codechick.io") || offer["@type"] != "Offer" || offer["category"] != "Free" || err != nil || price != 0 || offer["priceCurrency"] != "RUB" || offer["availability"] != "https://schema.org/InStock" || instance["courseMode"] != "online" {
		return bad()
	}
	fullFree := 0
	for _, q := range faq.Questions {
		if faq.Type == "FAQPage" && q.Name == "Курс «"+title+"» бесплатный?" && q.Answer.Text == "Да, полностью. Все уроки, упражнения и экзамен бесплатны — без скрытых платежей и подписок." {
			fullFree++
		}
	}
	syllabus := learningOne(doc, func(n *html.Node) bool { return classToken(n, "lp-syllabus") })
	lessons := learningNodes(syllabus, func(n *html.Node) bool { return classToken(n, "lp-syllabus-title") })
	starts := learningNodes(doc, func(n *html.Node) bool {
		return n.Data == "a" && classToken(n, "lp-cta") && strings.HasPrefix(nodeAttr(n, "href"), "/paths/"+c.ExternalID+"/")
	})
	freeLabel := learningOne(doc, func(n *html.Node) bool { return classToken(n, "lp-facts-price") })
	policyLabel := learningOne(doc, func(n *html.Node) bool { return classToken(n, "lp-facts-sub") })
	if fullFree != 1 || syllabus == nil || len(lessons) < 3 || len(starts) == 0 || freeLabel == nil || nodeText(freeLabel) != "Бесплатно" || policyLabel == nil || nodeText(policyLabel) != "навсегда, без подписок" {
		return bad()
	}
	o := smallFreeObservation(now)
	// ISO courseWorkload denotes total study effort, never weekly or calendar time.
	approximate := false
	for _, label := range learningNodes(doc, func(n *html.Node) bool { return classToken(n, "lp-section-sub") || classToken(n, "lp-facts-list") }) {
		text := nodeText(label)
		if strings.Contains(text, "~") || strings.Contains(text, "около") || strings.Contains(text, "≈") {
			approximate = true
		}
	}
	if match := regexp.MustCompile(`^PT([1-9][0-9]{0,3})H$`).FindStringSubmatch(schemaString(instance, "courseWorkload")); match != nil && !approximate {
		hours, _ := strconv.ParseFloat(match[1], 64)
		o.LearningConditions = &catalog.LearningConditions{TotalStudyHours: &catalog.LearningRange{Min: hours, Max: hours, Unit: "hour", Evidence: learningProof(data, c.URL, "Course.hasCourseInstance.courseWorkload (total study hours)", now)}}
	}
	r, err := normalizedRecord(c, title, schemaString(schema, "description"), providerNames[c.Adapter], o, now)
	if err != nil {
		return r, o, err
	}
	audience := learningOne(doc, func(n *html.Node) bool { return classToken(n, "lp-audience-text") })
	p := &catalog.CourseProfile{}
	if audience != nil {
		proof := learningProof(data, c.URL, ".lp-audience-text", now)
		p.Audience = profileTags(nodeText(audience), proof, "audience")
		p.Prerequisites = profileRequirement(nodeText(audience), r.Language, proof)
	}
	r, o = profileAttach(r, o, p)
	return r, o, nil
}

// Only the site's own Russian basics and advanced chapter lists are included.
// Children/sponsor courses and external certificates are independent products.
func learnPythonScope(data []byte) (string, string, error) {
	doc := learningDocument(data)
	russian, freeDescription := false, false
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "html" {
			russian = nodeAttr(n, "lang") == "ru"
		}
		if n.Data == "meta" && nodeAttr(n, "name") == "description" {
			freeDescription = nodeAttr(n, "content") == "learnpython.org is a free interactive Python tutorial for people who want to learn Python, fast."
		}
	})
	root := learningOne(doc, func(n *html.Node) bool { return n.Data == "div" && nodeAttr(n, "id") == "inner-text" })
	if !russian || !freeDescription || root == nil {
		return "", "", ErrSource
	}
	heading := learningOne(root, func(n *html.Node) bool { return n.Data == "h1" })
	if heading == nil || nodeText(heading) != "Добро пожаловать" {
		return "", "", ErrSource
	}
	intro := "Добро пожаловать в интерактивный учебник по Python на LearnPython.org."
	introNode := learningOne(root, func(n *html.Node) bool { return n.Data == "p" && nodeText(n) == intro })
	if introNode == nil {
		return "", "", ErrSource
	}
	sections := map[string]bool{}
	parts := []string{intro}
	base, _ := url.Parse("https://www.learnpython.org/ru/")
	for n := root.FirstChild; n != nil; n = n.NextSibling {
		if n.Data != "h3" {
			continue
		}
		title := nodeText(n)
		if title != "Изучите основы" && title != "Продвинутые уроки" {
			continue
		}
		if sections[title] {
			return "", "", ErrSource
		}
		sections[title] = true
		list := n.NextSibling
		for list != nil && list.Type != html.ElementNode {
			list = list.NextSibling
		}
		if list == nil || list.Data != "ul" {
			return "", "", ErrSource
		}
		links := learningNodes(list, func(n *html.Node) bool { return n.Data == "a" })
		if len(links) < 3 {
			return "", "", ErrSource
		}
		parts = append(parts, title)
		for _, link := range links {
			target, e := url.Parse(nodeAttr(link, "href"))
			if e != nil {
				return "", "", ErrSource
			}
			target = base.ResolveReference(target)
			if target.Scheme != "https" || target.Host != "www.learnpython.org" || !strings.HasPrefix(target.Path, "/ru/") || target.User != nil || target.RawQuery != "" || target.Fragment != "" || nodeText(link) == "" {
				return "", "", ErrSource
			}
			parts = append(parts, nodeText(link))
		}
	}
	if len(sections) != 2 {
		return "", "", ErrSource
	}
	return intro, strings.Join(parts, "\n"), nil
}

func collectLearnPython(data []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	if c.Adapter != "learnpython" || c.ExternalID != "python" || c.URL != "https://www.learnpython.org/ru" || len(data) > maxBody {
		return catalog.Course{}, Observation{}, ErrSource
	}
	intro, _, err := learnPythonScope(data)
	if err != nil {
		return catalog.Course{}, Observation{}, err
	}
	o := smallFreeObservation(now)
	r, err := normalizedRecord(c, "Интерактивный учебник Python", intro, providerNames[c.Adapter], o, now)
	return r, o, err
}

func aiSmallFreeDocuments(raw []byte, c Candidate, remote catalog.Course) (string, map[string]string, error) {
	if c.Adapter == "learnpython" {
		_, text, err := learnPythonScope(raw)
		return text, map[string]string{}, err
	}
	doc := learningDocument(raw)
	main := learningOne(doc, func(n *html.Node) bool { return n.Data == "main" })
	if c.Adapter != "codechick" || main == nil {
		return "", nil, ErrSource
	}
	// Purpose-built learning blocks only: avoid navigation, billing UI or footer.
	parts := []string{remote.Title, remote.Summary}
	for _, node := range learningNodes(main, func(n *html.Node) bool {
		return classToken(n, "lp-audience-text") || classToken(n, "lp-learn-grid") || classToken(n, "lp-syllabus")
	}) {
		parts = append(parts, nodeText(node))
	}
	return strings.Join(parts, "\n"), map[string]string{}, nil
}

func init() {
	aiProviders["codechick"] = "https://codechick.io/paths/%"
	aiProviders["learnpython"] = "https://www.learnpython.org/ru"
	aiBoundReaders["codechick"] = aiSmallFreeDocuments
	aiBoundReaders["learnpython"] = aiSmallFreeDocuments
}
