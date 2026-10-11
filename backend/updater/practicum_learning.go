package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"time"
)

func practicumLearning(page, profession []byte, c Candidate, slug string, p practicumProfession, now time.Time) *catalog.LearningConditions {
	conditions := &catalog.LearningConditions{}
	if months := practicumDurationMonths(p); months != nil {
		locator := "professions[slug=" + slug + "].duration (months)"
		if p.DurationUnit != nil {
			locator = "professions[slug=" + slug + "].catalog_duration_amount/unit"
		}
		conditions.Duration = &catalog.LearningRange{Min: float64(*months), Max: float64(*months), Unit: "month", Evidence: learningProof(profession, practicumProfessionURL(slug), locator, now)}
	}
	doc := learningDocument(page)
	// The parent landing's general feature cards do not establish plus support.
	// Only the exact purchasable SKU's card may supply support/workload facts.
	card := learningOne(doc, func(n *html.Node) bool {
		return classToken(n, "common-flow-card__wrapper") && nodeAttr(n, "id") == slug
	})
	if card != nil {
		proof := learningProof(page, c.URL, ".common-flow-card__wrapper#"+slug, now)
		support := learningSupportText(nodeText(card), proof)
		conditions.Review, conditions.Mentor, conditions.Community = support.Review, support.Mentor, support.Community
		conditions.WeeklyWorkload = learningUniqueRange(learningNodes(card, func(n *html.Node) bool { return n.Data == "p" || n.Data == "li" }), true, proof)
	}
	return conditions
}
