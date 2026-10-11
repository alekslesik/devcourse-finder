package catalog

import (
	"testing"
	"time"
)

func TestProfileFacetsPreserveOtherFiltersAndCountCourses(t *testing.T) {
	now := time.Now().UTC()
	base := fixture(now)
	base.Language, base.Direction = "python", "backend"
	base.Audience, base.Goals = []string{"none", "switch"}, []string{"job"}
	unknown := base
	unknown.ID, unknown.Audience, unknown.Goals, unknown.Direction = "unknown", []string{"unknown"}, []string{"unknown"}, "unknown"
	other := base
	other.ID, other.Language = "java", "java"
	closed := base
	closed.ID, closed.Offers = "closed", append([]Offer(nil), base.Offers...)
	for i := range closed.Offers {
		closed.Offers[i].Enrollment = "closed"
	}
	archived := base
	archived.ID, archived.Status = "archived", "archived"
	cs := []Course{base, unknown, other, closed, archived}
	f := Filter{Language: "python", Experience: "basics", Page: 900, PageSize: 1}
	facets := ProfileFacets(cs, f, now)
	exp := facets["experience"]
	if exp.Eligible != 2 || exp.Unknown != 1 || exp.Counts["none"] != 1 || exp.Counts["switch"] != 1 || exp.Counts["basics"] != 0 || exp.Selected != "basics" {
		t.Fatal(exp)
	}
	if facets["goal"].Eligible != 0 || facets["direction"].Eligible != 0 {
		t.Fatal("other selected filter was dropped", facets)
	}
	if len(Search(cs, f, now)) != 0 {
		t.Fatal("diagnosis relaxed strict search")
	}
	f.Experience = ""
	f.Goal = "job"
	exp = ProfileFacets(cs, f, now)["experience"]
	if exp.Eligible != 1 || exp.Unknown != 0 {
		t.Fatal("goal scope lost", exp)
	}
	f.Goal = ""
	f.IncludeClosed = true
	if got := ProfileFacets(cs, f, now)["experience"]; got.Eligible != 3 {
		t.Fatal("closed flag lost", got)
	}
	f.IncludeClosed = false
	f.Max = ptr(int64(1))
	if got := ProfileFacets(cs, f, now)["experience"]; got.Eligible != 0 {
		t.Fatal("budget lost", got)
	}
}

func TestProfileFacetCountsAgreeWithSearch(t *testing.T) {
	now := time.Now().UTC()
	var courses []Course
	for i, audience := range [][]string{{"none"}, {"basics", "projects"}, {"working", "switch"}, {"unknown"}, nil} {
		c := fixture(now)
		c.ID = string(rune('a' + i))
		c.Audience = audience
		courses = append(courses, c)
	}
	f := Filter{Experience: "working"}
	exp := ProfileFacets(courses, f, now)["experience"]
	if exp.Eligible != 5 || exp.Unknown != 2 {
		t.Fatal(exp)
	}
	for value, count := range exp.Counts {
		f.Experience = value
		if want := len(Search(courses, f, now)); count != want {
			t.Fatalf("%s: facet=%d search=%d", value, count, want)
		}
	}
}
