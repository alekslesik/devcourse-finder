package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
)

func init() {
	aiProviders["hexlet"] = "https://ru.hexlet.io/programs/%"
	aiBoundReaders["hexlet"] = aiHexletDocuments
}
func aiHexletDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	node, e := aiStructuredNode(raw, c)
	if e != nil {
		return "", nil, e
	}
	text := aiPublicText(schemaString(node, "description")) + " " + aiCourseSections(raw, c)
	offer, _ := node["offers"].(map[string]any)
	if list, ok := node["offers"].([]any); ok && len(list) == 1 {
		offer, _ = list[0].(map[string]any)
	}
	offers := map[string]string{}
	// Aggregate/minimum prices cannot identify a purchasable tier. Course profile
	// may still be enriched; no subscription/other program conditions are copied.
	if offer != nil && offer["@type"] == "Offer" && schemaString(offer, "name") != "" {
		name := schemaString(offer, "name")
		scope := learningOne(learningDocument(raw), func(n *html.Node) bool {
			return classToken(n, "tn-molecule") && strings.HasPrefix(nodeText(n), name+" ") && len(nodeText(n)) < 2000
		})
		if scope != nil {
			offers[r.Offers[0].ID] = aiSchemaCalendar(node) + " " + aiOfferText(scope)
		}
	}
	return text, offers, nil
}
