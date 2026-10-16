package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
)

func init() {
	aiProviders["rsschool"] = "https://rs.school/courses/%"
	aiBoundReaders["rsschool"] = aiRSDocuments
}
func aiRSDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	doc := learningDocument(raw)
	// Course/catalog identity, Russian language and cohort event are independently
	// checked natively. Stage lengths and later-stage benefits are conditional and
	// are intentionally absent from the offer document supplied to the model.
	community := learningOne(doc, func(n *html.Node) bool {
		return n.Data == "section" && strings.Contains(nodeText(n), "Here is link for the course Discord server")
	})
	offers := map[string]string{}
	if community != nil {
		offers[r.Offers[0].ID] = aiOfferText(community)
	}
	return aiCourseSections(raw, c), offers, nil
}
