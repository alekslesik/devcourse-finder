package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"time"
)

func stepikLearning(data []byte, course stepikCourse, c Candidate, now time.Time) *catalog.LearningConditions {
	source := candidateEndpoint(c)
	facts := &catalog.LearningConditions{}
	facts.WeeklyWorkload = learningTextRange(course.Workload, true, learningProof(data, source, "courses[id="+c.ExternalID+"].workload", now))
	// is_self_paced is scheduling, not a promise of no support; automatic task
	// checks, discussion counts, subscriptions and enrollment dates prove none of
	// the human support/calendar duration fields.
	doc := learningDocument([]byte(course.Description))
	scope := learningOne(doc, func(n *html.Node) bool {
		if n.Data != "section" {
			return false
		}
		h := learningNodes(n, func(x *html.Node) bool { return x.Data == "h2" })
		return len(h) == 1 && nodeText(h[0]) == "Поддержка на курсе"
	})
	if scope != nil {
		support := learningSupportText(nodeText(scope), learningProof(data, source, "courses[id="+c.ExternalID+"].description/section[h2=Поддержка на курсе]", now))
		facts.Review, facts.Mentor, facts.Community = support.Review, support.Mentor, support.Community
	}
	return facts
}
