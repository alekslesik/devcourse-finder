package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
	"time"
)

func rsLearning(data []byte, doc *html.Node, c Candidate, now time.Time) *catalog.LearningConditions {
	facts := &catalog.LearningConditions{}
	// Stage lengths and mentoring after qualifying are conditional. Registration
	// dates, alumni six-month history and automatic/peer checks cannot supply facts
	// for the full course offered to every entrant.
	sections := learningNodes(doc, func(n *html.Node) bool { return n.Data == "section" })
	var scope *html.Node
	for _, section := range sections {
		text := nodeText(section)
		if strings.Contains(text, "Here is link for the course Discord server") {
			if scope != nil {
				return facts
			}
			scope = section
		}
	}
	if scope != nil {
		proof := learningProof(data, c.URL, "section[p=Here is link for the course Discord server]", now)
		facts.Community = &catalog.LearningSupport{Available: true, Evidence: proof}
	}
	return facts
}
