package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"time"
)

func academyLearning(data []byte, c Candidate, now time.Time) *catalog.LearningConditions {
	doc := learningDocument(data)
	sections := learningNodes(doc, func(n *html.Node) bool { return classToken(n, "intensive-weeks-section") })
	var scope *html.Node
	for _, n := range sections {
		headings := learningNodes(n, func(x *html.Node) bool { return x.Data == "h3" })
		if len(headings) == 1 && nodeText(headings[0]) == "Индивидуальный формат" {
			if scope != nil {
				return nil
			}
			scope = n
		}
	}
	if scope == nil {
		return nil
	}
	proof := learningProof(data, c.URL, ".intensive-weeks-section[h3=Индивидуальный формат]", now)
	facts := learningSupportText(nodeText(scope), proof)
	facts.Duration = learningUniqueRange(learningNodes(scope, func(n *html.Node) bool { return n.Data == "li" }), false, proof)
	// A weekly one-hour mentor consultation is support, not study workload.
	return facts
}
