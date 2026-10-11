package updater

import (
	"devcourse-finder/catalog"
	"time"
)

func basicsLearning(data []byte, p basicsPage, c Candidate, now time.Time) *catalog.LearningConditions {
	// The Inertia landing object was verified against course.id/slug; sibling
	// navigation/footer landings and the platform-wide free policy are not facts.
	facts := learningSupportText(p.Props.Landing.Description, learningProof(data, c.URL, "props.courseLandingPage.description", now))
	facts.Duration = learningTextRange(p.Props.Landing.Duration, false, learningProof(data, c.URL, "props.courseLandingPage.duration", now))
	// Current duration is e.g. '27 часов': total curriculum effort, not a calendar
	// length or weekly workload. AI help and automatic checks are not human support.
	return facts
}
