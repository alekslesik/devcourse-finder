package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
	"time"
)

// Called only after the enclosing course identity, canonical and tariff contract
// have been verified. Use this course's fields, never testimonials, difficulty
// numbers, recommendations or a language guessed from a course title.
func purpleProfile(data []byte, course purpleCourse, c Candidate, language string, now time.Time) *catalog.CourseProfile {
	p := profileFromSections(data, c, language, now, profileContracts["purpleschool"])
	proof := func(field string) catalog.LearningEvidence {
		return learningProof(data, c.URL, "course[alias="+c.ExternalID+"]."+field, now)
	}
	if p.Prerequisites == nil {
		raw := strings.Join(course.Requirements, "; ")
		if profileText(raw) == "требований нет" {
			p.Prerequisites = &catalog.ProfileRequirement{Level: "none", Scope: "programming", Evidence: proof("requirements")}
		} else {
			p.Prerequisites = profileRequirement(raw, language, proof("requirements"))
		}
	}
	if p.Audience == nil {
		p.Audience = profileTags(course.Description, proof("description"), "audience")
		text := profileText(course.Description)
		if p.Audience == nil && strings.HasPrefix(text, "мы начнём с самого нуля и научимся") && !profileHas(text, "не подходит", "не для", "только после", "если", "возможно", "например") {
			p.Audience = &catalog.ProfileTags{Values: []string{"none"}, Evidence: proof("description")}
		}
	}
	if p.ExcludedAudience == nil {
		p.ExcludedAudience = profileExcludedAudience(course.Description, proof("description"))
	}
	if p.Goals == nil {
		p.Goals = profileTags(course.Description, proof("description"), "goals")
		// This explicit, recorded purpose is stronger than 'basics' or 'from zero'.
		// Match the complete paragraph so negation/conditions cannot sneak through.
		intro := "этот курс расчитан на тех, что хочет познакомиться с python и попробовать его на практике"
		doc := learningDocument([]byte(course.Description))
		if p.Goals == nil && language == "python" {
			paragraphs := learningNodes(doc, func(n *html.Node) bool { return n.Data == "p" })
			for _, paragraph := range paragraphs {
				if profileText(nodeText(paragraph)) == intro {
					p.Goals = &catalog.ProfileTags{Values: []string{"try"}, Evidence: proof("description")}
				}
			}
		}
	}
	return p
}
