package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"time"
)

func javaRushLearning(data []byte, card *html.Node, c Candidate, now time.Time) *catalog.LearningConditions {
	// The selected Premium purchase card is the complete scope. University and
	// Mentor Pro are separate SKUs. A MONTH subscription is never a course length.
	proof := learningProof(data, c.URL, ".payment-plan-card[title=Java Premium] .payment-plan-card__info-inner", now)
	info := learningOne(card, func(n *html.Node) bool { return classToken(n, "payment-plan-card__info-inner") })
	if info == nil {
		return nil
	}
	facts := learningSupportText(nodeText(info), proof)
	facts.WeeklyWorkload = learningUniqueRange(learningNodes(info, func(n *html.Node) bool { return n.Data == "li" || n.Data == "p" }), true, proof)
	return facts
}
