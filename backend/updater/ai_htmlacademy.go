package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

func init() {
	aiProviders["htmlacademy"] = "https://htmlacademy.ru/intensive/%"
	aiBoundReaders["htmlacademy"] = aiAcademyDocuments
}
func aiAcademyDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	var selected *html.Node
	for _, n := range learningNodes(learningDocument(raw), func(n *html.Node) bool { return classToken(n, "intensive-weeks-section") }) {
		headings := learningNodes(n, func(x *html.Node) bool { return x.Data == "h3" })
		if len(headings) == 1 && nodeText(headings[0]) == "Индивидуальный формат" {
			if selected != nil {
				return "", nil, ErrAI
			}
			selected = n
		}
	}
	// Native verification separately reads the exact public payment format.
	return aiCourseSections(raw, c), map[string]string{r.Offers[0].ID: aiOfferText(selected)}, nil
}
