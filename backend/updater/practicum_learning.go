package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
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

	// Course-level manual project review applies to the base program. A plus SKU
	// can inherit it only when its verified card explicitly includes the base.
	inherited := card != nil && (strings.Contains(strings.ToLower(nodeText(card)), "то же самое +") || strings.Contains(strings.ToLower(nodeText(card)), "включает базовый тариф"))
	if slug == c.ExternalID || inherited {
		features := learningOne(doc, func(n *html.Node) bool { return classToken(n, "mentors-section") })
		if features != nil && conditions.Review == nil {
			locator := ".mentors-section[base=" + c.ExternalID + "]"
			if slug != c.ExternalID {
				locator += "+.common-flow-card__wrapper#" + slug + "[includes-base]"
			}
			facts := learningSupportText(nodeText(features), learningProof(page, c.URL, locator, now))
			conditions.Review = facts.Review
		}
	}
	return conditions
}
