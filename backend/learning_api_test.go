package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestLearningConditionsSearchContractAndPrivateOwnership(t *testing.T) {
	repository := httpFixture()
	var offer = catalog.Offer{ID: "learning", Name: "Learning", Price: value(int64(2000000)), PriceKind: "exact", PriceCheckedAt: time.Now().UTC(), Enrollment: "continuous", Schedule: "flexible", URL: repository.courses[0].Source, SupportKnown: value(false), LearningOwnership: 31, DurationCollected: true}
	e := catalog.LearningEvidence{Source: offer.URL, Locator: "course.conditions", Digest: strings.Repeat("a", 64), CheckedAt: time.Now().UTC()}
	offer.LearningConditions = &catalog.LearningConditions{Duration: &catalog.LearningRange{Min: 3, Max: 5, Unit: "month", Evidence: e}, WeeklyWorkload: &catalog.LearningRange{Min: 8, Max: 12, Unit: "hours_per_week", Evidence: e}, Review: &catalog.LearningSupport{Available: true, Evidence: e}}
	repository.courses[0].Offers = []catalog.Offer{offer}
	handler := newHandler(repository)
	contract := newResponseContract(t)
	for _, tc := range []struct {
		query string
		total int
	}{{"support=review&hours=12", 1}, {"support=review&hours=10", 0}, {"support=mentor", 0}, {"support=self", 0}} {
		response := request(handler, "GET", "/api/v1/courses?"+tc.query, "")
		contract.check(t, response, "/api/v1/courses", "GET", 200)
		var body struct {
			Total int              `json:"total"`
			Items []catalog.Result `json:"items"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Total != tc.total {
			t.Fatal(tc, body)
		}
		if tc.total == 1 && (body.Items[0].Offer.LearningConditions.Duration.Unit != "month" || body.Items[0].Offer.LearningConditions.Duration.Min != 3 || body.Items[0].Offer.LearningConditions.Duration.Max != 5 || body.Items[0].Offer.LearningConditions.Mentor != nil) {
			t.Fatal("ranges or unknown support changed", body)
		}
		if strings.Contains(response.Body.String(), "learning_ownership") || strings.Contains(response.Body.String(), "duration_collected") {
			t.Fatal("private ownership leaked")
		}
	}
}
