package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
)

func init() {
	aiProviders["skypro"] = "https://sky.pro/course/programming/%"
	aiBoundReaders["skypro"] = aiSkyproDocuments
}
func aiSkyproDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	doc := learningDocument(raw)
	// The native collector verified a single target-product pricing section. Only
	// that section can speak for support; generic school/job promises are excluded.
	pricing := learningOne(doc, func(n *html.Node) bool {
		return n.Data == "div" && classToken(n, "t-rec") && strings.Contains(nodeText(n), "Стоимость и варианты оплаты")
	})
	hero := learningOne(doc, func(n *html.Node) bool { return n.Data == "h1" })
	for hero != nil && !classToken(hero, "t-rec") {
		hero = hero.Parent
	}
	return aiCourseSections(raw, c), map[string]string{r.Offers[0].ID: aiOfferText(hero) + " " + aiOfferText(pricing)}, nil
}
