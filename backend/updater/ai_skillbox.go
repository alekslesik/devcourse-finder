package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strconv"
)

func init() {
	aiProviders["skillbox"] = "https://skillbox.ru/course/%"
	aiBoundReaders["skillbox"] = aiSkillboxDocuments
}
func aiSkillboxDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	doc := learningDocument(raw)
	offers := map[string]string{}
	program := learningOne(doc, func(n *html.Node) bool { return n.Data == "section" && nodeAttr(n, "id") == "program" })
	duration := ""
	for _, n := range learningNodes(program, func(n *html.Node) bool { return classToken(n, "program-v6__stat") }) {
		duration += " " + aiOfferText(n)
	}
	seen := map[int64]bool{}
	var invalid bool
	visitNodes(doc, func(n *html.Node) {
		if n.Data != "li" || nodeAttr(n, "data-tariff-data") == "" {
			return
		}
		var t struct {
			ID int64 `json:"nomenclature_id"`
		}
		if decodeJSON([]byte(nodeAttr(n, "data-tariff-data")), &t) != nil || t.ID <= 0 || seen[t.ID] {
			invalid = true
			return
		}
		seen[t.ID] = true
		// Only the matched tier's feature list, never payment options or siblings.
		features := learningOne(n, func(x *html.Node) bool { return classToken(x, "tariffs-v5__features-inner") })
		offers[r.ID+"-t"+strconv.FormatInt(t.ID, 10)] = aiOfferText(features) + " " + duration
	})
	if invalid {
		return "", nil, ErrAI
	}
	return aiCourseSections(raw, c), offers, nil
}
