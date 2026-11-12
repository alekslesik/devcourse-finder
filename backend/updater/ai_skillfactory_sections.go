package updater

import (
	"strings"

	"golang.org/x/net/html"
)

// The independently verified PDEV course now renders its own curriculum in
// tn-molecule wrappers, also used by unrelated pricing cards. Select its exact
// semantic learning sections instead of deleting every wrapper on the page.
func aiSkillfactoryCurrentSections(raw []byte, c Candidate) string {
	if c.Adapter != "skillfactory" || c.ExternalID != "python-developer" {
		return ""
	}
	doc := learningDocument(raw)
	var remove []*html.Node
	visitNodes(doc, func(n *html.Node) {
		if n.Parent != nil && (profileBlockedNode(n) || n.Data == "form") {
			remove = append(remove, n)
		}
	})
	for _, node := range remove {
		if node.Parent != nil {
			node.Parent.RemoveChild(node)
		}
	}
	var texts []string
	for _, heading := range []string{"Чем занимается Python-разработчик", "Где пригодится Python", "Что изучите за 10 месяцев", "Что сможете указать в резюме после обучения"} {
		if scope := aiSemanticSection(doc, []string{heading}); scope != nil {
			texts = append(texts, nodeText(scope))
		}
	}
	return aiNormalize(strings.Join(texts, "\n"))
}
