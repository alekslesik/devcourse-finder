package catalog

import (
	"slices"
	"testing"
	"time"
)

func TestHardFiltersAndUnknownValues(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		filter Filter
		change func(*Course)
		want   string
	}{
		{name: "language mismatch", filter: Filter{Language: "python"}},
		{name: "direction mismatch", filter: Filter{Direction: "frontend"}},
		{name: "goal mismatch", filter: Filter{Goal: "job"}},
		{name: "audience mismatch", filter: Filter{Experience: "none"}},
		{name: "experienced audience", filter: Filter{Experience: "experienced"}, want: "self"},
		{name: "beginner not experienced", filter: Filter{Experience: "experienced"}, change: func(c *Course) { c.Audience = []string{"none"} }},
		{name: "unknown audience", filter: Filter{Experience: "switch"}, change: func(c *Course) { c.Audience = nil }},
		{name: "inclusive full price range", filter: Filter{Min: ptr(int64(2000000)), Max: ptr(int64(2000000))}, want: "self"},
		{name: "one-sided minimum", filter: Filter{Min: ptr(int64(3000000))}, want: "review"},
		{name: "one-sided maximum", filter: Filter{Max: ptr(int64(2000000))}, want: "self"},
		{name: "below budget", filter: Filter{Max: ptr(int64(1999999))}},
		{name: "unknown hours excluded", filter: Filter{Hours: ptr(8)}, want: "review"},
		{name: "hours exceed limit", filter: Filter{Hours: ptr(7)}},
		{name: "review", filter: Filter{Support: "review"}, want: "review"},
		{name: "review does not imply mentor", filter: Filter{Support: "mentor"}},
		{name: "mentor is independent", filter: Filter{Support: "mentor"}, change: func(c *Course) { c.Offers[0].Mentor = true }, want: "self"},
		{name: "self excludes human support", filter: Filter{Support: "self"}, want: "self"},
		{name: "flexible schedule", filter: Filter{Schedule: "flexible"}, want: "self"},
		{name: "scheduled classes", filter: Filter{Schedule: "scheduled"}, want: "review"},
		{name: "unknown schedule excluded", filter: Filter{Schedule: "scheduled"}, change: func(c *Course) { c.Offers[1].Schedule = "unknown" }},
		{name: "closed enrollment excluded", change: func(c *Course) {
			for i := range c.Offers {
				c.Offers[i].Enrollment = "closed"
			}
		}},
		{name: "unknown enrollment excluded", change: func(c *Course) {
			for i := range c.Offers {
				c.Offers[i].Enrollment = "unknown"
			}
		}},
		{name: "closed enrollment explicitly allowed", filter: Filter{IncludeClosed: true}, change: func(c *Course) {
			for i := range c.Offers {
				c.Offers[i].Enrollment = "closed"
			}
		}, want: "self"},
		{name: "unknown enrollment explicitly allowed", filter: Filter{IncludeClosed: true}, change: func(c *Course) {
			for i := range c.Offers {
				c.Offers[i].Enrollment = "unknown"
			}
		}, want: "self"},
		{name: "archived", change: func(c *Course) { c.Status = "archived" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			course := fixture(now)
			course.Direction = "backend"
			course.Offers[1].Schedule = "scheduled"
			if test.change != nil {
				test.change(&course)
			}
			found := Search([]Course{course}, test.filter, now)
			if test.want == "" {
				if len(found) != 0 {
					t.Fatalf("nonmatching course passed: %v", found)
				}
				return
			}
			if len(found) != 1 || found[0].Offer.ID != test.want {
				t.Fatalf("wrong tariff for %s: %v", test.name, found)
			}
		})
	}
}
func TestFreeTariffsAndSelection(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	course := fixture(now)
	free := course.Offers[0]
	free.ID = "free"
	free.Free = true
	free.Price = ptr(int64(0))
	course.Offers = append(course.Offers, free)
	for _, test := range []struct {
		name   string
		filter Filter
		want   string
	}{
		{"free wins without restrictions", Filter{}, "free"},
		{"only free", Filter{Budget: "free"}, "free"},
		{"paid excludes free", Filter{Budget: "paid"}, "self"},
		{"paid explicitly includes free", Filter{Budget: "paid", IncludeFree: true}, "free"},
		{"paid minimum", Filter{Min: ptr(int64(1000000))}, "self"},
		{"explicit free bypasses paid minimum", Filter{Min: ptr(int64(1000000)), IncludeFree: true}, "free"},
	} {
		t.Run(test.name, func(t *testing.T) {
			found := Search([]Course{course}, test.filter, now)
			if len(found) != 1 || found[0].Offer.ID != test.want {
				t.Fatalf("wrong free/paid choice: %v", found)
			}
		})
	}
	course.Offers[2].PriceCheckedAt = now.Add(-31 * 24 * time.Hour)
	if len(Search([]Course{course}, Filter{Budget: "free"}, now)) != 0 {
		t.Fatal("unconfirmed free tariff passed")
	}
	course = fixture(now)
	course.Offers[1].Price = course.Offers[0].Price
	if got := Search([]Course{course}, Filter{}, now); got[0].Offer.ID != "review" {
		t.Fatal("equal prices must use stable offer ID")
	}
	for i := range course.Offers {
		course.Offers[i].Price = nil
		course.Offers[i].PriceKind = "unknown"
	}
	if got := Search([]Course{course}, Filter{}, now); len(got) != 1 || got[0].Offer.ID != "review" || got[0].Price != nil {
		t.Fatal("unknown prices must use stable offer ID when budget is unrestricted")
	}
}
func TestPriceValidityBoundaries(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	offer := fixture(now).Offers[0]
	offer.PriceCheckedAt = now.Add(-30 * 24 * time.Hour)
	if EffectivePrice(offer, now) == nil {
		t.Fatal("price exactly 30 days old should still be valid")
	}
	offer.PriceCheckedAt = offer.PriceCheckedAt.Add(-time.Nanosecond)
	if EffectivePrice(offer, now) != nil {
		t.Fatal("price older than 30 days passed")
	}
	offer.PriceCheckedAt = now
	offer.ValidUntil = &now
	if EffectivePrice(offer, now) != nil {
		t.Fatal("price valid_until is exclusive")
	}
	offer.ValidUntil = ptr(now.Add(time.Nanosecond))
	if EffectivePrice(offer, now) == nil {
		t.Fatal("unexpired price excluded")
	}
}
func TestSearchOrderingAndStaleness(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	makeCourse := func(id string, price *int64, weeks *int, age time.Duration) Course {
		course := fixture(now)
		course.ID = id
		course.CheckedAt = now.Add(-age)
		course.Offers = course.Offers[:1]
		course.Offers[0].Price = price
		course.Offers[0].Weeks = weeks
		if price == nil {
			course.Offers[0].PriceKind = "unknown"
		}
		return course
	}
	courses := []Course{makeCourse("d", ptr(int64(2000000)), ptr(8), 2*time.Hour), makeCourse("c", nil, nil, 91*24*time.Hour), makeCourse("a", ptr(int64(2000000)), ptr(12), time.Hour), makeCourse("b", ptr(int64(1000000)), ptr(8), 0)}
	for _, test := range []struct {
		sort string
		want []string
	}{{"recent", []string{"b", "a", "d", "c"}}, {"price_asc", []string{"b", "a", "d", "c"}}, {"price_desc", []string{"a", "d", "b", "c"}}, {"duration", []string{"b", "d", "a", "c"}}} {
		t.Run(test.sort, func(t *testing.T) {
			found := Search(courses, Filter{Sort: test.sort}, now)
			ids := []string{}
			for _, result := range found {
				ids = append(ids, result.Course.ID)
				if result.Course.ID == "c" && !result.Stale {
					t.Fatal("outdated conditions not marked")
				}
			}
			if !slices.Equal(ids, test.want) {
				t.Fatalf("order %v, want %v", ids, test.want)
			}
		})
	}
	course := fixture(now)
	course.CheckedAt = now.Add(-90 * 24 * time.Hour)
	if Search([]Course{course}, Filter{}, now)[0].Stale {
		t.Fatal("conditions exactly 90 days old should not yet be stale")
	}
	course.CheckedAt = course.CheckedAt.Add(-time.Nanosecond)
	if !Search([]Course{course}, Filter{}, now)[0].Stale {
		t.Fatal("conditions older than 90 days must be marked")
	}
}
