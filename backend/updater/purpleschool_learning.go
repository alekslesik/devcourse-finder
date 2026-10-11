package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strconv"
	"strings"
	"time"
)

func purpleLearning(data []byte, doc *html.Node, p purpleCourse, r catalog.Course, o Observation, c Candidate, now time.Time) (catalog.Course, Observation) {
	by := map[string]*catalog.LearningConditions{}
	faq := learningOne(doc, func(n *html.Node) bool {
		return classPrefix(n, "FAQ-module_") && strings.Contains(nodeAttr(n, "class"), "__sections")
	})
	var weekly *catalog.LearningRange
	if faq != nil {
		cards := learningNodes(faq, func(n *html.Node) bool {
			return classPrefix(n, "Card-module_") && strings.Contains(nodeAttr(n, "class"), "__card")
		})
		var target []*html.Node
		for _, card := range cards {
			if strings.HasPrefix(nodeText(card), "Сколько нужно выделить времени на курс?") {
				target = append(target, card)
			}
		}
		if len(target) == 1 {
			weekly = learningTextRange(nodeText(target[0]), true, learningProof(data, c.URL, "FAQ[question=Сколько нужно выделить времени на курс?]", now))
		}
	}
	for _, t := range p.Tariffs {
		id := r.ID + "-t" + strconv.FormatInt(t.ID, 10)
		proof := learningProof(data, c.URL, learningLocator("course.tariffs.features", t.ID), now)
		facts := learningSupportText(strings.Join(t.Features, " "), proof)
		facts.WeeklyWorkload = weekly
		// AI feedback is not human review; a direct mentor chat is community support,
		// not an individual appointment. No absence inferred from an empty feature list.
		by[id] = facts
	}
	return learningAttach(r, o, by)
}
