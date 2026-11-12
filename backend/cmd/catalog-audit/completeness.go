package main

import (
	"sort"
	"time"

	"devcourse-finder/catalog"
)

// These are disjoint course partitions, not sums of overlapping option counts.
type facetPartition struct {
	Confirmed    []string `json:"confirmed_ids"`
	Recommended  []string `json:"recommended_only_ids"`
	Unclassified []string `json:"unclassified_ids"`
}

type coverageGap struct {
	CourseID string   `json:"course_id"`
	Provider string   `json:"provider"`
	Language string   `json:"language"`
	Source   string   `json:"source"`
	Fields   []string `json:"missing_fields"`
}

type completenessReport struct {
	Courses    int                                   `json:"courses"`
	Fields     map[string]*facetPartition            `json:"fields"`
	ByProvider map[string]map[string]*facetPartition `json:"by_provider"`
	Backlog    []coverageGap                         `json:"backlog"`
	Accounted  bool                                  `json:"all_courses_accounted"`
}

func newPartition() *facetPartition {
	return &facetPartition{Confirmed: []string{}, Recommended: []string{}, Unclassified: []string{}}
}

func summarizeCompleteness(courses []catalog.Course, now time.Time) completenessReport {
	r := completenessReport{Courses: len(courses), Fields: map[string]*facetPartition{}, ByProvider: map[string]map[string]*facetPartition{}, Backlog: []coverageGap{}, Accounted: true}
	for _, f := range auditFacets {
		r.Fields[f.key] = newPartition()
	}
	seen := map[string]bool{}
	for _, c := range courses {
		if c.ID == "" || seen[c.ID] {
			r.Accounted = false
			continue
		}
		seen[c.ID] = true
		if r.ByProvider[c.Provider] == nil {
			r.ByProvider[c.Provider] = map[string]*facetPartition{}
			for _, f := range auditFacets {
				r.ByProvider[c.Provider][f.key] = newPartition()
			}
		}
		gap := coverageGap{CourseID: c.ID, Provider: c.Provider, Language: c.Language, Source: c.Source, Fields: []string{}}
		for _, f := range auditFacets {
			confirmed, recommended := false, false
			for _, value := range f.values {
				confirmed = confirmed || catalog.MatchProfile(c, f.key, value) == "confirmed"
				recommended = recommended || catalog.MatchRecommendation(c, f.key, value, now)
			}
			for _, p := range []*facetPartition{r.Fields[f.key], r.ByProvider[c.Provider][f.key]} {
				switch {
				case confirmed:
					p.Confirmed = append(p.Confirmed, c.ID)
				case recommended:
					p.Recommended = append(p.Recommended, c.ID)
				default:
					p.Unclassified = append(p.Unclassified, c.ID)
				}
			}
			if !confirmed && !recommended {
				gap.Fields = append(gap.Fields, f.key)
			}
		}
		if len(gap.Fields) > 0 {
			r.Backlog = append(r.Backlog, gap)
		}
	}
	for _, p := range r.Fields {
		if len(p.Confirmed)+len(p.Recommended)+len(p.Unclassified) != r.Courses {
			r.Accounted = false
		}
	}
	for _, fields := range append([]map[string]*facetPartition{r.Fields}, providerPartitions(r.ByProvider)...) {
		for _, p := range fields {
			sort.Strings(p.Confirmed)
			sort.Strings(p.Recommended)
			sort.Strings(p.Unclassified)
		}
	}
	// Most missing fields first; ties are stable and independent of pagination.
	sort.Slice(r.Backlog, func(i, j int) bool {
		if len(r.Backlog[i].Fields) != len(r.Backlog[j].Fields) {
			return len(r.Backlog[i].Fields) > len(r.Backlog[j].Fields)
		}
		return r.Backlog[i].CourseID < r.Backlog[j].CourseID
	})
	return r
}

func providerPartitions(providers map[string]map[string]*facetPartition) []map[string]*facetPartition {
	out := []map[string]*facetPartition{}
	for _, fields := range providers {
		out = append(out, fields)
	}
	return out
}
