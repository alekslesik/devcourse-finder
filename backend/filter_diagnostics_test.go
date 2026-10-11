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
