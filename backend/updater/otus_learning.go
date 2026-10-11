package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"regexp"
	"strconv"
	"time"
)

var learningISOCalendar = regexp.MustCompile(`^P([1-9][0-9]*)(D|W|M|Y)$`)

func learningISODuration(raw string, evidence catalog.LearningEvidence) *catalog.LearningRange {
	m := learningISOCalendar.FindStringSubmatch(raw)
	if m == nil {
		return nil
	}
	n, _ := strconv.ParseFloat(m[1], 64)
	unit := map[string]string{"D": "day", "W": "week", "M": "month", "Y": "year"}[m[2]]
	v := &catalog.LearningRange{Min: n, Max: n, Unit: unit, Evidence: evidence}
	if !(&catalog.LearningConditions{Duration: v}).Valid() {
		return nil
	}
	return v
}
func otusLearning(data []byte, node map[string]any, c Candidate, now time.Time) *catalog.LearningConditions {
	facts := &catalog.LearningConditions{}
	// Calendar duration belongs to the accepted CourseInstance. Dates can include
	// vacations; enrollment window and PT video/workload totals are not duration.
	if instance, ok := node["hasCourseInstance"].(map[string]any); ok && instance["@type"] == "CourseInstance" {
		facts.Duration = learningISODuration(schemaString(instance, "duration"), learningProof(data, c.URL, "Course.hasCourseInstance.duration", now))
	}
	doc := learningDocument(data)
	about := learningOne(doc, func(n *html.Node) bool { return nodeAttr(n, "id") == "vdescription" })
	if about != nil {
		proof := learningProof(data, c.URL, "#vdescription p", now)
		facts.WeeklyWorkload = learningUniqueRange(learningNodes(about, func(n *html.Node) bool { return n.Data == "p" }), true, proof)
		support := learningSupportText(nodeText(about), proof)
		facts.Review, facts.Mentor, facts.Community = support.Review, support.Mentor, support.Community
	}
	return facts
}
