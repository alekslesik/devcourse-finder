package main

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"fmt"
	"testing"
)

func TestDiscoveryHTTPPaginationAndSubjectFlagAreConsistent(t *testing.T) {
	repo := httpFixture()
	base := repo.courses[0]
	repo.courses = nil
	for n := 0; n < 45; n++ {
		c := base
		c.ID = fmt.Sprintf("rank-%02d", n)
		c.Slug = c.ID
		c.Provider = []string{"Яндекс Практикум", "OTUS", "Хекслет", "Stepik", "Skillbox"}[n%5]
		c.Title = "Обычный курс"
		if n%4 == 0 {
			c.Title = "Курс по LLM и нейросетям"
		}
		c.Offers = append([]catalog.Offer(nil), base.Offers...)
		for j := range c.Offers {
			c.Offers[j].ID = fmt.Sprintf("%s-offer-%d", c.ID, j)
		}
		repo.courses = append(repo.courses, c)
	}
	handler := newHandler(repo)
	contract := newResponseContract(t)
	seen := map[string]bool{}
	for page := 1; page <= 3; page++ {
		response := request(handler, "GET", fmt.Sprintf("/api/v1/courses?sort=discovery&page_size=18&page=%d", page), "")
		contract.check(t, response, "/api/v1/courses", "GET", 200)
		var body struct {
			Items    []catalog.Result
			Total    int
			PageSize int `json:"page_size"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Total != 45 || body.PageSize != 18 {
			t.Fatal(body)
		}
		for _, item := range body.Items {
			if seen[item.Course.ID] {
				t.Fatal("repeated course across pages", item.Course.ID)
			}
			seen[item.Course.ID] = true
			if item.AICourse != catalog.IsAICourse(item.Course) {
				t.Fatal("inconsistent subject flag")
			}
		}
	}
	if len(seen) != 45 {
		t.Fatal("pagination lost courses", len(seen))
	}
	response := request(handler, "GET", "/api/v1/courses?sort=recent", "")
	var body struct {
		PageSize int `json:"page_size"`
	}
	json.Unmarshal(response.Body.Bytes(), &body)
	if body.PageSize != 12 {
		t.Fatal("API default page size changed")
	}
}
