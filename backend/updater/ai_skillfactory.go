package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

func init() {
	aiProviders["skillfactory"] = "https://skillfactory.ru/%"
	aiBoundReaders["skillfactory"] = aiSkillfactoryDocuments
}
func aiSkillfactoryDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	doc := learningDocument(raw)
	header := learningOne(doc, func(n *html.Node) bool { return classToken(n, "program__header") })
	// The pricing API binds tiers but exposes no tier-specific support. Lead popups
	// are never support evidence. Only the explicit common program length is shared.
	labels := learningNodes(header, func(n *html.Node) bool { return classToken(n, "program__label") })
	text := ""
	for _, n := range labels {
		text += " " + aiOfferText(n)
	}
	offers := map[string]string{}
	for _, o := range r.Offers {
		offers[o.ID] = text
	}
	return aiCourseSections(raw, c), offers, nil
}
