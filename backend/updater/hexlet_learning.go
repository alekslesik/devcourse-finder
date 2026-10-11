package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
	"time"
)

func hexletLearning(data []byte, node map[string]any, c Candidate, now time.Time) *catalog.LearningConditions {
	facts := &catalog.LearningConditions{}
	if instance, ok := node["hasCourseInstance"].(map[string]any); ok && instance["@type"] == "CourseInstance" {
		facts.Duration = learningISODuration(schemaString(instance, "duration"), learningProof(data, c.URL, "Course.hasCourseInstance.duration", now))
	}
	// A landing has several tiers. DOM conditions require the accepted schema's
	// exact named single Offer; anonymous/minimum offers cannot bind a tier.
	offer, _ := node["offers"].(map[string]any)
	if list, ok := node["offers"].([]any); ok && len(list) == 1 {
		offer, _ = list[0].(map[string]any)
	}
	if offer == nil || offer["@type"] != "Offer" || schemaString(offer, "name") == "" {
		return facts
	}
	name := schemaString(offer, "name")
	scope := learningOne(learningDocument(data), func(n *html.Node) bool {
		return classToken(n, "tn-molecule") && strings.HasPrefix(nodeText(n), name+" ") && len(nodeText(n)) < 2000
	})
	if scope == nil {
		return facts
	}
	proof := learningProof(data, c.URL, ".tn-molecule[Offer.name="+name+"]", now)
	nodes := learningNodes(scope, func(n *html.Node) bool { return classToken(n, "tn-atom") })
	if facts.Duration == nil {
		facts.Duration = learningUniqueRange(nodes, false, proof)
	}
	facts.WeeklyWorkload = learningUniqueRange(nodes, true, proof)
	support := learningSupportText(nodeText(scope), proof)
	facts.Review, facts.Mentor, facts.Community = support.Review, support.Mentor, support.Community
	return facts
}
