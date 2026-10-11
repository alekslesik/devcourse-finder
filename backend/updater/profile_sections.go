package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
	"time"
)

type profileHeadings struct{ Audience, Requirements, Outcomes []string }

func profileBlockedNode(n *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p.Data == "nav" || p.Data == "footer" || p.Data == "script" || p.Data == "template" || p.Data == "noscript" {
			return true
		}
		cls := strings.ToLower(nodeAttr(p, "class"))
		if profileHas(cls, "recommend", "related-course", "course-card", "tariffs", "testimonial") {
			return true
		}
	}
	return false
}
func profileSection(doc *html.Node, headings []string) (*html.Node, string) {
	var found *html.Node
	var foundRoot *html.Node
	label := ""
	ambiguous := false
	visitNodes(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || profileBlockedNode(n) || !(n.Data == "h2" || n.Data == "h3" || n.Data == "h4" || n.Data == "p" || n.Data == "strong" || n.Data == "button" || (n.Data == "div" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode && n.FirstChild.NextSibling == nil)) {
			return
		}
		text := normalizeTitle(nodeText(n))
		matched := false
		for _, h := range headings {
			if text == normalizeTitle(h) {
				matched = true
				break
			}
		}
		if !matched {
			return
		}
		for parent, depth := n.Parent, 0; parent != nil && depth < 8; parent, depth = parent.Parent, depth+1 {
			if parent.Data == "body" || parent.Data == "html" || profileBlockedNode(parent) {
				return
			}
			content := nodeText(parent)
			if len(content) <= len(nodeText(n))+20 {
				continue
			}
			if len(content) > 12000 {
				return
			}
			// End this semantic section at the next heading; adjacent promotional
			// sections do not establish this course audience or requirements.
			start := strings.Index(content, nodeText(n))
			if start >= 0 {
				content = content[start:]
			}
			visitNodes(parent, func(x *html.Node) {
				if x != n && (x.Data == "h2" || x.Data == "h3") && nodeText(x) != nodeText(n) {
					if at := strings.Index(content, nodeText(x)); at > 0 {
						content = content[:at]
					}
				}
			})
			if len(content) <= len(nodeText(n))+20 {
				return
			}
			if foundRoot != nil && foundRoot != parent {
				ambiguous = true
			}
			foundRoot = parent
			found = &html.Node{Type: html.ElementNode, Data: "section"}
			found.AppendChild(&html.Node{Type: html.TextNode, Data: content})
			label = text
			return
		}
	})
	if ambiguous {
		return nil, ""
	}
	return found, label
}
func profileFromSections(data []byte, c Candidate, language string, now time.Time, h profileHeadings) *catalog.CourseProfile {
	doc := learningDocument(data)
	p := &catalog.CourseProfile{}
	extract := func(headings []string) (string, catalog.LearningEvidence) {
		n, label := profileSection(doc, headings)
		if n == nil {
			return "", catalog.LearningEvidence{}
		}
		return nodeText(n), learningProof(data, c.URL, "course-section[heading="+label+"]", now)
	}
	text, proof := extract(h.Audience)
	p.Audience = profileTags(text, proof, "audience")
	text, proof = extract(h.Requirements)
	p.Prerequisites = profileRequirement(text, language, proof)
	text, proof = extract(h.Outcomes)
	p.Goals = profileTags(text, proof, "goals")
	p.Direction = profileDirection(text, proof)
	return p
}
func stepikDescriptionProfile(data []byte, course stepikCourse, c Candidate, now time.Time, p *catalog.CourseProfile) {
	doc := learningDocument([]byte(course.Description))
	for _, kind := range []string{"audience", "requirements", "outcomes"} {
		headings := map[string][]string{"audience": {"Для кого этот курс", "Для кого этот курс?", "Кому подойдет курс"}, "requirements": {"Требования к обучению", "Что нужно знать перед курсом"}, "outcomes": {"Чему вы научитесь", "Результаты обучения"}}[kind]
		n, label := profileSection(doc, headings)
		if n == nil {
			continue
		}
		proof := learningProof(data, candidateEndpoint(c), "courses[id="+c.ExternalID+"].description[heading="+label+"]", now)
		text := nodeText(n)
		switch kind {
		case "audience":
			if p.Audience == nil {
				p.Audience = profileTags(text, proof, kind)
			}
		case "requirements":
			if p.Prerequisites == nil {
				p.Prerequisites = profileRequirement(text, courseLanguage(course.Title, course.Summary), proof)
			}
		case "outcomes":
			p.Goals = profileTags(text, proof, "goals")
			if p.Direction == nil {
				p.Direction = profileDirection(text, proof)
			}
		}
	}
}
