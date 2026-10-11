package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"time"
)

func skillfactoryLearning(data []byte, doc *html.Node, r catalog.Course, o Observation, c Candidate, now time.Time) (catalog.Course, Observation) {
	// Program header is course-specific. Tilda recommendation cards, campaign
	// timers, academic-hour totals and unrelated hidden popups are excluded.
	header := learningOne(doc, func(n *html.Node) bool { return classToken(n, "program__header") })
	proof := learningProof(data, c.URL, ".program__header .program__label", now)
	duration := learningUniqueRange(learningNodes(header, func(n *html.Node) bool { return classToken(n, "program__label") }), false, proof)
	by := map[string]*catalog.LearningConditions{}
	for _, offer := range r.Offers {
		by[offer.ID] = &catalog.LearningConditions{Duration: duration}
	}
	// Pricing GET provides no learning/support flags. A popup is a lead form, not
	// evidence that a consultation is personal or included in the purchased tier.
	return learningAttach(r, o, by)
}
