package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestAuditAllPagesAndUnknownMatches(t *testing.T) {
	for _, fault := range []string{"", "unknown_count", "duplicate_page", "changed_snapshot", "recommended_count"} {
		t.Run(fault, func(t *testing.T) {
			now := time.Now().UTC()
			zero := int64(0)
			courses := []catalog.Course{}
			for i := 0; i < 50; i++ {
				audience := []string{"unknown"}
				if i < 4 {
					audience = []string{"none"}
				} else if i < 8 {
					audience = []string{"working"}
				}
				courses = append(courses, catalog.Course{ID: fmt.Sprint(i), Slug: fmt.Sprint(i), Title: "Python", Language: "python", Source: "https://example.com/python", Status: "published", CheckedAt: now, Audience: audience, Goals: []string{"unknown"}, Offers: []catalog.Offer{{ID: fmt.Sprint(i), Free: true, Price: &zero, PriceKind: "exact", PriceCheckedAt: now, Enrollment: "continuous"}}})
			}
			evidence := catalog.LearningEvidence{Source: "https://example.com/python", Locator: "program", Digest: strings.Repeat("a", 64), CheckedAt: now}
			courses[8].Recommendations = &catalog.CourseRecommendations{Audience: &catalog.RecommendationTags{Values: []string{"none", "basics"}, Quote: "Curriculum starts with basic programming", Evidence: evidence}, Goals: &catalog.RecommendationTags{Values: []string{"try", "job"}, Quote: "Curriculum includes first practical projects", Evidence: evidence}, Directions: &catalog.RecommendationTags{Values: []string{"basics", "backend"}, Quote: "Curriculum includes HTTP and API projects", Evidence: evidence}}
			changed := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				f, err := catalog.Parse(r.URL.Query())
				if err != nil {
					t.Fatal(err)
				}
				if f.Experience != "" || f.Goal != "" {
					changed = true
				}
				cs := append([]catalog.Course(nil), courses...)
				if fault == "changed_snapshot" && changed {
					cs[0].Title = "Changed Python"
				}
				items := catalog.Search(cs, f, now)
				confirmed, unknown, recommended := 0, 0, 0
				for _, x := range items {
					if x.ProfileMatch == "unverified" {
						unknown++
					} else if x.ProfileMatch == "recommended" {
						recommended++
					} else {
						confirmed++
					}
				}
				prospective := f
				prospective.IncludeUnknownProfiles = true
				available := 0
				for _, x := range catalog.Search(cs, prospective, now) {
					if x.ProfileMatch == "unverified" {
						available++
					}
				}
				start := min((f.Page-1)*f.PageSize, len(items))
				end := min(start+f.PageSize, len(items))
				page := items[start:end]
				if fault == "duplicate_page" && f.Page == 2 {
					page = []catalog.Result{items[0], items[1]}
				}
				if fault == "unknown_count" && f.Experience == "none" {
					available++
				}
				if fault == "recommended_count" && f.IncludeRecommendations {
					recommended++
				}
				_ = json.NewEncoder(w).Encode(response{Recommended: recommended, Items: page, Total: len(items), Confirmed: confirmed, Unverified: unknown, UnknownAvailable: available, Page: f.Page, PageSize: f.PageSize})
			}))
			defer server.Close()
			got, err := (auditor{client: server.Client(), base: server.URL}).run(context.Background())
			if fault != "" {
				if err == nil && got.Pass {
					t.Fatal("invalid audit passed", fault)
				}
				return
			}
			if err != nil || !got.Pass || !got.Stable || got.Courses != 50 || len(got.Rows) != 90 || len(got.Combinations) != 50 {
				t.Fatal(got, err)
			}
			if got.Coverage["confirmed"].Experience != 8 || got.Coverage["with_recommendations"].Experience != 9 || got.Coverage["with_recommendations"].Goal != 1 || got.Coverage["with_recommendations"].Direction != 1 || got.Coverage["with_recommendations"].AllThree != 1 {
				t.Fatal("coverage counted tags instead of distinct courses", got.Coverage)
			}
			r := got.Rows[0]
			if r.Confirmed != 4 || r.Unknown != 42 || r.Excluded != 4 || len(r.UnknownIDs) != 42 {
				t.Fatal(r)
			}
		})
	}
}
