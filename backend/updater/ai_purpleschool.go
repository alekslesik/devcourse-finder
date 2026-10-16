package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strconv"
	"strings"
)

func init() {
	aiProviders["purpleschool"] = "https://purpleschool.ru/course/%"
	aiBoundReaders["purpleschool"] = aiPurpleDocuments
}
func aiPurpleDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	doc := learningDocument(raw)
	data, e := purpleFlight(doc, c.ExternalID)
	if e != nil {
		return "", nil, e
	}
	var p purpleCourse
	if decodeJSON(data, &p) != nil {
		return "", nil, ErrAI
	}
	// Typed public fields of the verified course, never the whole Flight stream.
	text := aiPublicText(p.Description) + " Требования: " + aiPublicText(strings.Join(p.Requirements, " ")) + " " + aiCourseSections(raw, c)
	faq := learningOne(doc, func(n *html.Node) bool {
		return classPrefix(n, "FAQ-module_") && strings.Contains(nodeAttr(n, "class"), "__sections")
	})
	weekly := learningOne(faq, func(n *html.Node) bool {
		return classPrefix(n, "Card-module_") && strings.Contains(nodeAttr(n, "class"), "__card") && strings.HasPrefix(nodeText(n), "Сколько нужно выделить времени на курс?")
	})
	offers := map[string]string{}
	for _, t := range p.Tariffs {
		offers[r.ID+"-t"+strconv.FormatInt(t.ID, 10)] = aiPublicText(strings.Join(t.Features, " ")) + " " + aiOfferText(weekly)
	}
	return text, offers, nil
}
