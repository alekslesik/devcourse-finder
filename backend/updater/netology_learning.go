package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
	"time"
)

func netologyLearningScope(doc *html.Node, plans []netologyPlan, c Candidate, name string) *html.Node {
	var scopes []*html.Node
	visitNodes(doc, func(n *html.Node) {
		if n.Data != "p" || !cardField.MatchString(nodeAttr(n, "name")) || normalizeTitle(nodeText(n)) != normalizeTitle(name) {
			return
		}
		bound := false
		for _, p := range plans {
			for _, section := range p.Sections {
				for _, card := range section.Cards {
					if card.Slug == c.ExternalID && normalizeTitle(plainText(card.Title, 500)) == normalizeTitle(name) {
						bound = true
					}
				}
			}
		}
		if !bound {
			return
		}
		for parent := n.Parent; parent != nil; parent = parent.Parent {
			titles, payments, features := 0, 0, 0
			visitNodes(parent, func(x *html.Node) {
				if x.Data == "p" && cardField.MatchString(nodeAttr(x, "name")) {
					titles++
				}
				if x.Data == "p" && strings.Contains(nodeAttr(x, "name"), ".features[") {
					features++
				}
				if classPrefix(x, "styles_prices__") {
					payments++
				}
			})
			if titles > 1 {
				return
			}
			if titles == 1 && payments == 1 && features > 0 {
				scopes = append(scopes, parent)
				return
			}
		}
	})
	if len(scopes) != 1 {
		return nil
	}
	return scopes[0]
}

func netologyLearning(data []byte, doc *html.Node, plans []netologyPlan, c Candidate, name string, now time.Time) *catalog.LearningConditions {
	scope := netologyLearningScope(doc, plans, c, name)
	if scope == nil {
		return nil
	}
	proof := learningProof(data, c.URL, "plansNew/card[slug="+c.ExternalID+"]/rendered-features", now)
	nodes := learningNodes(scope, func(n *html.Node) bool {
		return n.Data == "p" && strings.Contains(nodeAttr(n, "name"), ".features[") && strings.HasSuffix(nodeAttr(n, "name"), ".title")
	})
	facts := learningSupportText(nodeText(scope), proof)
	for _, n := range nodes {
		text := strings.Split(strings.Split(nodeText(n), ", ")[0], " и ")[0]
		v := learningTextRange(text, false, proof)
		if v != nil {
			if facts.Duration != nil && (facts.Duration.Min != v.Min || facts.Duration.Max != v.Max) {
				facts.Duration = nil
				break
			}
			facts.Duration = v
		}
	}
	facts.WeeklyWorkload = learningUniqueRange(nodes, true, proof)
	return facts
}
