package catalog

import (
	"testing"
	"time"
)

// Exercise every language and all visible profile combinations against the same
// offer predicate, including unknowns. The assertions are behavioral invariants,
// not another copy of the search implementation.
func TestProfileFilterCombinationMatrix(t *testing.T) {
	now := time.Now()
	var courses []Course
	for _, language := range []string{"python", "javascript", "java", "go"} {
		for n, kind := range []string{"confirmed", "unknown", "closed", "stale-price"} {
			c := fixture(now)
			c.ID = language + kind
			c.Language = language
			c.Goals = []string{"job"}
			c.Direction = "backend"
			c.Profile = &CourseProfile{Prerequisites: &ProfileRequirement{Level: "none", Scope: "programming", Evidence: profileProof()}, TargetLevel: &ProfileValue{Value: "projects", Evidence: profileProof()}}
			c.Audience = []string{"unknown"}
			c.Offers = append([]Offer(nil), c.Offers[:1]...)
			c.Offers[0].ID = c.ID
			if n == 1 {
				c.Profile = nil
				c.Goals = []string{"unknown"}
				c.Direction = "unknown"
			}
			if n == 2 {
				c.Offers[0].Enrollment = "closed"
			}
			if n == 3 {
				c.Offers[0].PriceCheckedAt = now.Add(-31 * 24 * time.Hour)
			}
			courses = append(courses, c)
		}
	}
	for _, language := range []string{"python", "javascript", "java", "go"} {
		for _, experience := range append([]string{""}, experienceValues...) {
			for _, goal := range append([]string{""}, goalValues...) {
				for _, direction := range append([]string{""}, directionValues...) {
					f := Filter{Language: language, Experience: experience, Goal: goal, Direction: direction, Max: ptr(int64(3000000))}
					strict := Search(courses, f, now)
					f.IncludeUnknownProfiles = true
					expanded := Search(courses, f, now)
					if len(expanded) < len(strict) {
						t.Fatal("opt-in removed confirmed matches", f)
					}
					seenUnknown := false
					for _, r := range expanded {
						if r.Course.Language != language || r.Offer.Enrollment == "closed" || r.Price == nil || *r.Price > 3000000 {
							t.Fatal("unrelated constraint relaxed", f, r)
						}
						if r.ProfileMatch == "unverified" {
							seenUnknown = true
						} else if seenUnknown {
							t.Fatal("unknown precedes confirmed", f)
						}
						for _, pair := range [][2]string{{"experience", experience}, {"goal", goal}, {"direction", direction}} {
							if MatchProfile(r.Course, pair[0], pair[1]) == "incompatible" {
								t.Fatal("known mismatch admitted", f, r)
							}
						}
					}
					facets := ProfileFacets(courses, f, now)
					if len(facets) != 3 {
						t.Fatal("missing facets")
					}
				}
			}
		}
	}
}
