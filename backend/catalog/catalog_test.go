package catalog

import (
	"os"
	"slices"
	"testing"
	"time"
)

func ptr[T any](v T) *T { return &v }
func fixture(now time.Time) Course {
	return Course{ID: "go", Slug: "go", Title: "Go", Language: "go", Status: "published", Audience: []string{"switch"}, Goals: []string{"switch"}, CheckedAt: now, Offers: []Offer{
		{ID: "self", Price: ptr(int64(2000000)), PriceKind: "exact", PriceCheckedAt: now, Enrollment: "continuous", Schedule: "flexible"},
		{ID: "review", Price: ptr(int64(4000000)), PriceKind: "exact", PriceCheckedAt: now, Enrollment: "continuous", Review: true, Hours: ptr(8)},
	}}
}
func TestSameOfferMustMatchAllFilters(t *testing.T) {
	n := time.Now()
	c := fixture(n)
	if got := Search([]Course{c}, Filter{Support: "review", Max: ptr(int64(3000000))}, n); len(got) != 0 {
		t.Fatal("combined cheap price with expensive review")
	}
	got := Search([]Course{c}, Filter{Support: "review", Max: ptr(int64(5000000))}, n)
	if len(got) != 1 || got[0].Offer.ID != "review" {
		t.Fatal("matching review offer not selected")
	}
}
func TestPriceAndEnrollment(t *testing.T) {
	n := time.Now()
	for _, kind := range []string{"unknown", "from", "expired", "stale", "closed"} {
		t.Run(kind, func(t *testing.T) {
			c := fixture(n)
			c.Offers = c.Offers[:1]
			switch kind {
			case "unknown":
				c.Offers[0].Price = nil
			case "from":
				c.Offers[0].PriceKind = "from"
			case "expired":
				v := n.Add(-time.Hour)
				c.Offers[0].ValidUntil = &v
			case "stale":
				c.Offers[0].PriceCheckedAt = n.AddDate(0, 0, -31)
			case "closed":
				c.Offers[0].Enrollment = "closed"
			}
			if len(Search([]Course{c}, Filter{Max: ptr(int64(3000000))}, n)) != 0 {
				t.Fatal("unconfirmed price or closed enrollment passed")
			}
		})
	}
}
func TestFreeAndProfile(t *testing.T) {
	n := time.Now()
	c := fixture(n)
	c.Offers = c.Offers[:1]
	c.Offers[0].Free = true
	c.Offers[0].Price = ptr(int64(0))
	if len(Search([]Course{c}, Filter{Budget: "free", Experience: "switch"}, n)) != 1 {
		t.Fatal("free course missing")
	}
	if len(Search([]Course{c}, Filter{Experience: "none"}, n)) != 0 {
		t.Fatal("wrong prerequisite")
	}
	c.Status = "draft"
	if len(Search([]Course{c}, Filter{}, n)) != 0 {
		t.Fatal("draft exposed")
	}
}
func TestUnknownHoursAndStableOrder(t *testing.T) {
	n := time.Now()
	c := fixture(n)
	got := Search([]Course{c}, Filter{Hours: ptr(10)}, n)
	if len(got) != 1 || got[0].Offer.ID != "review" {
		t.Fatal("unknown hours passed or known hours excluded")
	}
	a := fixture(n)
	a.ID = "a"
	c.ID = "z"
	got = Search([]Course{c, a}, Filter{}, n)
	if len(got) != 2 || got[0].Course.ID != "a" {
		t.Fatal("unstable ordering")
	}
}

func TestDemoCatalogIsValidAndRepresentative(t *testing.T) {
	f, err := os.Open("../../data/demo-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	dataset, err := Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(dataset.Courses) != 20 {
		t.Fatalf("demo catalog has %d courses, want 20", len(dataset.Courses))
	}

	languages := map[string]int{}
	hasFree, hasPaid := false, false
	hasSelf, hasReview, hasMentor := false, false, false
	hasBeginner, hasExperienced := false, false
	for _, course := range dataset.Courses {
		if !course.Demo {
			t.Fatalf("course %q is not marked as demo", course.ID)
		}
		languages[course.Language]++
		if slices.Contains(course.Audience, "none") {
			hasBeginner = true
		}
		if slices.Contains(course.Audience, "working") {
			hasExperienced = true
		}
		for _, offer := range course.Offers {
			hasFree = hasFree || offer.Free
			hasPaid = hasPaid || !offer.Free
			hasReview = hasReview || offer.Review
			hasMentor = hasMentor || offer.Mentor
			hasSelf = hasSelf || (!offer.Review && !offer.Mentor)
		}
	}
	for _, language := range []string{"go", "python", "java", "javascript"} {
		if languages[language] != 5 {
			t.Errorf("demo catalog has %d %s courses, want 5", languages[language], language)
		}
	}
	if !hasFree || !hasPaid || !hasSelf || !hasReview || !hasMentor || !hasBeginner || !hasExperienced {
		t.Fatal("demo catalog does not cover all required price, support, and experience variants")
	}
}
