package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

func init() {
	aiProviders["javarush"] = "https://javarush.com/prices"
	aiBoundReaders["javarush"] = aiJavaRushDocuments
}
func aiJavaRushDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	doc := learningDocument(raw)
	card := learningOne(doc, func(n *html.Node) bool {
		if n.Data != "div" || nodeAttr(n, "class") != "payment-plan-card" {
			return false
		}
		title := learningOne(n, func(x *html.Node) bool { return classToken(x, "payment-plan-card__title") })
		return title != nil && nodeText(title) == "Java Premium"
	})
	info := learningOne(card, func(n *html.Node) bool { return classToken(n, "payment-plan-card__info-inner") })
	// Cost/period/buy widgets and University/Mentor Pro cards never enter AI input.
	// Independent monthly API is verified natively, with currency left unchanged.
	return aiCourseSections(raw, c) + " " + aiOfferText(info), map[string]string{r.Offers[0].ID: aiOfferText(info)}, nil
}
