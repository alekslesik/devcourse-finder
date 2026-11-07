package main

import (
	"context"
	"devcourse-finder/catalog"
	"net/url"
	"strconv"
	"time"
)

var auditFacets = []struct {
	key    string
	values []string
}{
	{"experience", []string{"none", "basics", "projects", "working", "switch", "experienced"}},
	{"goal", []string{"try", "job", "switch", "deepen"}},
	{"direction", catalog.SearchDirections()},
}

type profileCoverage struct {
	// Each course counts at most once per field, even with multiple matching tags.
	Experience int `json:"experience"`
	Goal       int `json:"goal"`
	Direction  int `json:"direction"`
	AllThree   int `json:"all_three"`
}

func summarizeCoverage(courses []catalog.Course, now time.Time) map[string]profileCoverage {
	out := map[string]profileCoverage{}
	for _, include := range []bool{false, true} {
		covered := profileCoverage{}
		for _, c := range courses {
			matched := [3]bool{}
			for n, facet := range auditFacets {
				for _, value := range facet.values {
					if catalog.MatchProfile(c, facet.key, value) == "confirmed" || include && catalog.MatchRecommendation(c, facet.key, value, now) {
						matched[n] = true
						break
					}
				}
			}
			if matched[0] {
				covered.Experience++
			}
			if matched[1] {
				covered.Goal++
			}
			if matched[2] {
				covered.Direction++
			}
			if matched[0] && matched[1] && matched[2] {
				covered.AllThree++
			}
		}
		key := "confirmed"
		if include {
			key = "with_recommendations"
		}
		out[key] = covered
	}
	return out
}

type combination struct {
	Filters  url.Values `json:"filters"`
	Expected int        `json:"expected"`
	APITotal int        `json:"api_total"`
	Match    bool       `json:"matches_backend"`
}

func resultCountsMatch(got response, expected []catalog.Result) bool {
	confirmed, recommended, unverified := 0, 0, 0
	for _, item := range expected {
		switch item.ProfileMatch {
		case "recommended":
			recommended++
		case "unverified":
			unverified++
		default:
			confirmed++
		}
	}
	return got.Total == len(expected) && got.Confirmed == confirmed && got.Recommended == recommended && got.Unverified == unverified
}

func (a auditor) auditCombinations(ctx context.Context, courses []catalog.Course, out *report) error {
	for _, language := range []string{"", "python", "javascript", "java", "go"} {
		for _, route := range [][3]string{{"none", "try", "basics"}, {"none", "job", "backend"}, {"basics", "job", "frontend"}, {"working", "deepen", "data"}, {"switch", "switch", "backend"}} {
			for _, unknown := range []bool{false, true} {
				q := url.Values{"language": {language}, "experience": {route[0]}, "goal": {route[1]}, "direction": {route[2]}, "include_recommendations": {"true"}, "include_unknown_profiles": {strconv.FormatBool(unknown)}}
				f, _ := catalog.Parse(q)
				expected := catalog.Search(courses, f, out.Started)
				api, err := a.get(ctx, q)
				if err != nil {
					return err
				}
				r := combination{Filters: q, Expected: len(expected), APITotal: api.Total, Match: resultCountsMatch(api, expected)}
				out.Combinations = append(out.Combinations, r)
				out.Pass = out.Pass && r.Match
			}
		}
	}
	return nil
}
