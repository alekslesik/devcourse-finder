package catalog

import (
	"slices"
	"time"
)

// Facet counts distinct eligible courses with this facet's own constraint
// removed. Other filters and offer eligibility still apply. Counts can overlap.
type Facet struct {
	Eligible int            `json:"eligible"`
	Unknown  int            `json:"unknown"`
	Selected string         `json:"selected"`
	Counts   map[string]int `json:"counts"`
}

func matchesExperience(audience []string, value string) bool {
	if value == "experienced" {
		for _, a := range audience {
			if slices.Contains([]string{"basics", "projects", "working", "switch"}, a) {
				return true
			}
		}
		return false
	}
	return slices.Contains(audience, value)
}

// ProfileFacets diagnoses missing metadata without relaxing search or treating
// unknown profiles as confirmed matches. Search returns one result per course.
func ProfileFacets(courses []Course, f Filter, now time.Time) map[string]Facet {
	out := map[string]Facet{}
	for _, key := range []string{"experience", "goal", "direction"} {
		base := f
		var selected string
		var values []string
		switch key {
		case "experience":
			selected, base.Experience = f.Experience, ""
			values = []string{"none", "basics", "projects", "working", "switch", "experienced"}
		case "goal":
			selected, base.Goal = f.Goal, ""
			values = []string{"try", "job", "switch", "deepen"}
		case "direction":
			selected, base.Direction = f.Direction, ""
			values = []string{"basics", "backend", "frontend", "fullstack", "automation"}
		}
		eligible := Search(courses, base, now)
		facet := Facet{Eligible: len(eligible), Selected: selected, Counts: map[string]int{}}
		for _, v := range values {
			facet.Counts[v] = 0
		}
		for _, result := range eligible {
			known := false
			for _, v := range values {
				matches := false
				switch key {
				case "experience":
					matches = matchesExperience(result.Course.Audience, v)
				case "goal":
					matches = slices.Contains(result.Course.Goals, v)
				case "direction":
					matches = result.Course.Direction == v
				}
				if matches {
					facet.Counts[v]++
					known = true
				}
			}
			if !known {
				facet.Unknown++
			}
		}
		out[key] = facet
	}
	return out
}
