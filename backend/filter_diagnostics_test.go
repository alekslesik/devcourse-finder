package main

import (
	"encoding/json"
	"net/http"
	"testing"

	"devcourse-finder/catalog"
)

func TestSearchDiagnosticsOptionalAndUnpaginated(t *testing.T) {
	repo := httpFixture()
	unknown := repo.courses[0]
	unknown.ID, unknown.Slug = "unknown-profile", "unknown-profile"
	unknown.Audience, unknown.Goals = []string{"unknown"}, []string{"unknown"}
	repo.courses = append(repo.courses, unknown)
	handler := newHandler(repo)
	response := request(handler, http.MethodGet, "/api/v1/courses?language=go&experience=basics&diagnostics=true&page=2&page_size=1", "")
	newResponseContract(t).check(t, response, "/api/v1/courses", "get", 200)
	var body struct {
		Total       int
		Items       []catalog.Result
		Diagnostics map[string]catalog.Facet
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 0 || len(body.Items) != 0 || body.Diagnostics["experience"].Eligible != 2 || body.Diagnostics["experience"].Unknown != 1 || body.Diagnostics["experience"].Counts["switch"] != 1 {
		t.Fatal(response.Body.String())
	}
	plain := request(handler, http.MethodGet, "/api/v1/courses?diagnostics=false", "")
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(plain.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, exists := raw["diagnostics"]; exists {
		t.Fatal("default search computed optional diagnostics")
	}
	invalid := request(handler, http.MethodGet, "/api/v1/courses?diagnostics=perhaps", "")
	if invalid.Code != 400 {
		t.Fatal(invalid.Code)
	}
}

func TestSearchZeroMinimumIsNeutral(t *testing.T) {
	repo := httpFixture()
	repo.courses[0].Offers = repo.courses[0].Offers[:1]
	repo.courses[0].Offers[0].Price = nil
	repo.courses[0].Offers[0].PriceKind = "unknown"
	handler := newHandler(repo)
	for _, query := range []string{"", "?min=0"} {
		response := request(handler, http.MethodGet, "/api/v1/courses"+query, "")
		newResponseContract(t).check(t, response, "/api/v1/courses", "get", 200)
		var body struct{ Total int }
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Total != 1 {
			t.Fatal(response.Body.String(), err)
		}
	}
	for _, query := range []string{"?min=1", "?min=0&max=1000000000"} {
		response := request(handler, http.MethodGet, "/api/v1/courses"+query, "")
		var body struct{ Total int }
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body.Total != 0 {
			t.Fatal(response.Body.String(), err)
		}
	}
}

func TestUnknownProfileResponseCountsPrecedePagination(t *testing.T) {
	repo := httpFixture()
	known := repo.courses[0]
	known.Audience = []string{"basics"}
	known.Goals = []string{"job"}
	known.Direction = "backend"
	unknown := known
	unknown.ID = "unknown"
	unknown.Slug = "unknown"
	unknown.Audience = []string{"unknown"}
	incompatible := known
	incompatible.ID = "incompatible"
	incompatible.Slug = "incompatible"
	incompatible.Goals = []string{"deepen"}
	repo.courses = []catalog.Course{unknown, incompatible, known}
	h := newHandler(repo)
	for _, include := range []bool{false, true} {
		query := "/api/v1/courses?experience=basics&goal=job&diagnostics=true&page=2&page_size=1"
		if include {
			query += "&include_unknown_profiles=true"
		}
		response := request(h, http.MethodGet, query, "")
		newResponseContract(t).check(t, response, "/api/v1/courses", "get", 200)
		var body struct {
			Total      int
			Confirmed  int `json:"confirmed_total"`
			Unverified int `json:"unverified_total"`
			Available  int `json:"unknown_available"`
			Items      []catalog.Result
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Confirmed != 1 || body.Available != 1 {
			t.Fatal(response.Body.String())
		}
		if include {
			if body.Total != 2 || body.Unverified != 1 || len(body.Items) != 1 || body.Items[0].ProfileMatch != "unverified" {
				t.Fatal(response.Body.String())
			}
		} else if body.Total != 1 || body.Unverified != 0 || len(body.Items) != 0 {
			t.Fatal(response.Body.String())
		}
	}
}
