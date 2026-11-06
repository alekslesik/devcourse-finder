package updater

import (
	"devcourse-finder/catalog"
	"time"
)

// Use the search matcher, rather than the presence of JSON keys: a stored
// "unknown" or an entry requirement alone is not a confirmed suitability match.
func aiProfileMissing(c catalog.Course) map[string]bool {
	values := map[string][]string{
		"experience": {"none", "basics", "projects", "working", "switch"},
		"goal":       {"try", "job", "switch", "deepen"},
		"direction":  {"basics", "backend", "frontend", "fullstack", "automation"},
	}
	out := map[string]bool{}
	for field, options := range values {
		out[field] = true
		for _, option := range options {
			if catalog.MatchProfile(c, field, option) == "confirmed" {
				out[field] = false
				break
			}
		}
	}
	return out
}

func aiMissingCount(c catalog.Course) int {
	n := 0
	for field, missing := range aiProfileMissing(c) {
		if missing && !aiHasRecommendation(c, field, time.Now()) {
			n++
		}
	}
	return n
}

func aiHasRecommendation(c catalog.Course, field string, now time.Time) bool {
	tags := c.Recommendations.Tags(field)
	if tags == nil {
		return false
	}
	for _, v := range tags.Values {
		if catalog.MatchRecommendation(c, field, v, now) {
			return true
		}
	}
	return false
}
