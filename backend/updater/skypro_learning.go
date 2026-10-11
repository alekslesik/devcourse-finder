package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"time"
)

func skyproLearning(data []byte, doc *html.Node, pricing *html.Node, c Candidate, now time.Time) *catalog.LearningConditions {
	var hero *html.Node
	titles := learningNodes(doc, func(n *html.Node) bool { return n.Data == "h1" })
	if len(titles) == 1 {
		for n := titles[0].Parent; n != nil; n = n.Parent {
			if classToken(n, "t-rec") {
				hero = n
				break
			}
		}
	}
	proof := learningProof(data, c.URL, ".t-rec[h1] .tn-atom", now)
	facts := learningSupportText(nodeText(pricing), learningProof(data, c.URL, ".t-rec[target-product pricing]", now))
	nodes := learningNodes(hero, func(n *html.Node) bool { return classToken(n, "tn-atom") })
	facts.Duration = learningUniqueRange(nodes, false, proof)
	facts.WeeklyWorkload = learningUniqueRange(nodes, true, proof)
	return facts
}
