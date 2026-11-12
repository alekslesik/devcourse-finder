package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestCompletenessPartitionsAllCoursesWithoutCountingTagsTwice(t *testing.T) {
	now := time.Now().UTC()
	evidence := catalog.LearningEvidence{Source: "https://example.com/python", Locator: "curriculum", Digest: strings.Repeat("a", 64), CheckedAt: now}
	advice := &catalog.CourseRecommendations{Audience: &catalog.RecommendationTags{Values: []string{"none", "basics"}, Quote: "Learn variables and first practical programs", Evidence: evidence}}
	courses := []catalog.Course{
		{ID: "known", Provider: "School A", Audience: []string{"none", "basics"}, Recommendations: advice},
		{ID: "recommended", Provider: "School B", Audience: []string{"unknown"}, Recommendations: advice},
		{ID: "unknown", Provider: "School A", Audience: []string{"unknown"}},
		{ID: "expired", Provider: "School B", Audience: []string{"unknown"}, Recommendations: &catalog.CourseRecommendations{Audience: &catalog.RecommendationTags{Values: []string{"none"}, Quote: advice.Audience.Quote, Evidence: evidence}}},
	}
	courses[3].Recommendations.Audience.Evidence.CheckedAt = now.Add(-91 * 24 * time.Hour)
	r := summarizeCompleteness(courses, now)
	p := r.Fields["experience"]
	if !r.Accounted || !reflect.DeepEqual(p.Confirmed, []string{"known"}) || !reflect.DeepEqual(p.Recommended, []string{"recommended"}) || !reflect.DeepEqual(p.Unclassified, []string{"expired", "unknown"}) {
		t.Fatalf("wrong partition: %+v", r)
	}
	for field, p := range r.Fields {
		if len(p.Confirmed)+len(p.Recommended)+len(p.Unclassified) != 4 {
			t.Fatal("course disappeared", field, p)
		}
		for provider, fields := range r.ByProvider {
			x := fields[field]
			if len(x.Confirmed)+len(x.Recommended)+len(x.Unclassified) != 2 {
				t.Fatal("provider partition lost a course", provider, field)
			}
		}
	}
	if len(r.Backlog) != 4 || r.Backlog[0].CourseID != "expired" || r.Backlog[1].CourseID != "unknown" {
		t.Fatal("missing fields not prioritized", r.Backlog)
	}
	// Input ordering and page boundaries must not change the report.
	courses[0], courses[3] = courses[3], courses[0]
	if !reflect.DeepEqual(r, summarizeCompleteness(courses, now)) {
		t.Fatal("pagination order changed completeness")
	}
}

func TestCompletenessRejectsDuplicateOrMissingIdentities(t *testing.T) {
	for _, courses := range [][]catalog.Course{{{ID: "a"}, {ID: "a"}}, {{ID: ""}}} {
		if summarizeCompleteness(courses, time.Now()).Accounted {
			t.Fatal("invalid identities accepted")
		}
	}
	if !summarizeCompleteness(nil, time.Now()).Accounted {
		t.Fatal("empty catalog should be accounted")
	}
}
