package main

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSearchRecommendationContractCountsPaginationAndOptIn(t *testing.T) {
	repo := httpFixture()
	now := time.Now().UTC()
	base := repo.courses[0]
	tags := func(v string) *catalog.RecommendationTags {
		return &catalog.RecommendationTags{Values: []string{v}, Quote: "Изучаем язык, создаём проекты и практикуем разработку сервисов.", Evidence: catalog.LearningEvidence{Source: base.Source, Locator: "program", Digest: strings.Repeat("a", 64), CheckedAt: now}}
	}
	confirmed := base
	confirmed.ID = "confirmed"
	confirmed.Audience = []string{"none"}
	confirmed.Goals = []string{"job"}
	estimate := base
	estimate.ID = "estimate"
	estimate.Direction = "unknown"
	estimate.Audience = []string{"unknown"}
	estimate.Goals = []string{"unknown"}
	estimate.Recommendations = &catalog.CourseRecommendations{Audience: tags("none"), Goals: tags("job"), Directions: tags("backend")}
	unknown := estimate
	unknown.ID = "unknown"
	unknown.Recommendations = nil
	repo.courses = []catalog.Course{unknown, estimate, confirmed}
	handler := newHandler(repo)
	q := "/api/v1/courses?experience=none&goal=job&direction=backend&include_recommendations=true&include_unknown_profiles=true&diagnostics=true&page_size=1&page=2"
	r := request(handler, http.MethodGet, q, "")
	newResponseContract(t).check(t, r, "/api/v1/courses", "get", 200)
	var d struct {
		Total       int                      `json:"total"`
		Confirmed   int                      `json:"confirmed_total"`
		Recommended int                      `json:"recommended_total"`
		Unknown     int                      `json:"unverified_total"`
		Items       []catalog.Result         `json:"items"`
		Diagnostics map[string]catalog.Facet `json:"diagnostics"`
	}
	if err := json.Unmarshal(r.Body.Bytes(), &d); err != nil {
		t.Fatal(err)
	}
	if d.Total != 3 || d.Confirmed != 1 || d.Recommended != 1 || d.Unknown != 1 || len(d.Items) != 1 || d.Items[0].ProfileMatch != "recommended" || d.Items[0].Course.ID != "estimate" || d.Diagnostics["experience"].RecommendedCounts["none"] != 1 {
		t.Fatal(r.Body.String())
	}
	strict := request(handler, http.MethodGet, "/api/v1/courses?experience=none&goal=job&direction=backend", "")
	var plain struct {
		Total int `json:"total"`
	}
	json.Unmarshal(strict.Body.Bytes(), &plain)
	if plain.Total != 1 {
		t.Fatal("default API changed strict semantics", strict.Body.String())
	}
	budget := request(handler, http.MethodGet, q+"&max=1", "")
	json.Unmarshal(budget.Body.Bytes(), &plain)
	if plain.Total != 0 {
		t.Fatal("recommendations bypassed price", budget.Body.String())
	}
	invalid := request(handler, http.MethodGet, "/api/v1/courses?include_recommendations=yes", "")
	if invalid.Code != 400 {
		t.Fatal(invalid.Code)
	}
}
