package updater

import "devcourse-finder/catalog"

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
	for _, missing := range aiProfileMissing(c) {
		if missing {
			n++
		}
	}
	return n
}
