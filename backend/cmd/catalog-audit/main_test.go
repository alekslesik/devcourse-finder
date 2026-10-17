package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestAuditAllPagesAndUnknownMatches(t *testing.T) {
	for _, fault := range []string{"", "unknown_count", "duplicate_page", "changed_snapshot"} {
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
				confirmed, unknown := 0, 0
				for _, x := range items {
					if x.ProfileMatch == "unverified" {
						unknown++
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
				_ = json.NewEncoder(w).Encode(response{Items: page, Total: len(items), Confirmed: confirmed, Unverified: unknown, UnknownAvailable: available, Page: f.Page, PageSize: f.PageSize})
			}))
			defer server.Close()
			got, err := (auditor{client: server.Client(), base: server.URL}).run(context.Background())
			if fault != "" {
				if err == nil && got.Pass {
					t.Fatal("invalid audit passed", fault)
				}
				return
			}
			if err != nil || !got.Pass || !got.Stable || got.Courses != 50 || len(got.Rows) != 40 {
				t.Fatal(got, err)
			}
			r := got.Rows[0]
			if r.Confirmed != 4 || r.Unknown != 42 || r.Excluded != 4 || len(r.UnknownIDs) != 42 {
				t.Fatal(r)
			}
		})
	}
}
