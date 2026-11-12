package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
)

func init() {
	aiProviders["skypro"] = "https://sky.pro/course/programming/%"
	aiBoundReaders["skypro"] = aiSkyproDocuments
}
func aiSkyproDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	doc := learningDocument(raw)
	// The native collector verified a single target-product pricing section. Only
	// that section can speak for support; generic school/job promises are excluded.
	pricing := learningOne(doc, func(n *html.Node) bool {
		return n.Data == "div" && classToken(n, "t-rec") && strings.Contains(nodeText(n), "Стоимость и варианты оплаты")
	})
	hero := learningOne(doc, func(n *html.Node) bool { return n.Data == "h1" })
	for hero != nil && !classToken(hero, "t-rec") {
		hero = hero.Parent
	}
	text := aiCourseSections(raw, c)
	if curriculum := aiSkyproCurrentCurriculum(doc, c); curriculum != "" {
		text = aiNormalize(text + "\n" + curriculum)
	}
	return text, map[string]string{r.Offers[0].ID: aiOfferText(hero) + " " + aiOfferText(pricing)}, nil
}

// The observed React curriculum starts at its own heading record and continues
// through adjacent module records. Preserve their nested lesson text without
// broadening selection to the whole page, salary blocks or other courses.
func aiSkyproCurrentCurriculum(doc *html.Node, c Candidate) string {
	if c.Adapter != "skypro" || c.ExternalID != "react-developer" {
		return ""
	}
	var scope *html.Node
	ambiguous := false
	visitNodes(doc, func(n *html.Node) {
		if n.Data != "h2" || normalizeTitle(nodeText(n)) != normalizeTitle("Программа курса") || skyproCurriculumBlocked(n) {
			return
		}
		root := n.Parent
		for root != nil && root.Data != "body" && root.Data != "html" && !classToken(root, "t-rec") {
			root = root.Parent
		}
		if root == nil || !classToken(root, "t-rec") || skyproCurriculumBlocked(root) {
			return
		}
		headings := 0
		visitNodes(root, func(x *html.Node) {
			if x.Data == "h2" && !skyproCurriculumBlocked(x) {
				headings++
			}
		})
		if headings != 1 || scope != nil {
			ambiguous = true
			return
		}
		scope = root
	})
	if ambiguous || scope == nil {
		return ""
	}
	var text strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if skyproCurriculumBlocked(n) || n.Data == "form" || n.Data == "input" || n.Data == "button" {
			return
		}
		if n.Type == html.TextNode {
			text.WriteString(n.Data)
			text.WriteByte(' ')
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	// Tilda puts each module heading and its accordion in separate sibling
	// records. Stop before the next section or the observed consultation CTA.
	bounded, modules, records := false, 0, 0
	for block := scope; block != nil && records < 24; block = block.NextSibling {
		if block.Type != html.ElementNode {
			continue
		}
		if skyproCurriculumBlocked(block) {
			if block.Data == "script" || block.Data == "style" {
				continue
			}
			return ""
		}
		if block.Data != "div" || !classToken(block, "t-rec") {
			return ""
		}
		records++
		end := false
		visitNodes(block, func(n *html.Node) {
			if skyproCurriculumBlocked(n) {
				return
			}
			if block != scope && n.Data == "h2" {
				end = true
			}
			if n.Data == "h3" && normalizeTitle(nodeText(n)) == normalizeTitle("Получите полную программу на личной консультации") {
				end = true
			}
		})
		if end {
			bounded = true
			break
		}
		visitNodes(block, func(n *html.Node) {
			if n.Data == "h3" && !skyproCurriculumBlocked(n) && strings.HasPrefix(nodeText(n), "Модуль ") {
				modules++
			}
		})
		walk(block)
		if text.Len() > 24000 {
			return ""
		}
	}
	result := aiNormalize(text.String())
	if !bounded || modules == 0 || len(result) < 80 || len(result) > 24000 {
		return ""
	}
	return result
}

// Boundaries and text traversal use the same ancestor-aware exclusions.
func skyproCurriculumBlocked(n *html.Node) bool {
	if profileBlockedNode(n) {
		return true
	}
	for p := n; p != nil; p = p.Parent {
		if p.Data == "form" || p.Data == "input" || p.Data == "button" || nodeAttr(p, "data-tariff-data") != "" || classToken(p, "payment-plan-card") || classToken(p, "intensive-weeks-section") || classPrefix(p, "styles_prices__") {
			return true
		}
	}
	return false
}
