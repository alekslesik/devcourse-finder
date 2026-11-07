package updater

import (
	"encoding/json"
	"testing"
)

func TestAIReviewIndicesAndRulesAreIndependentOfQuoteSegments(t *testing.T) {
	claims := []AIClaim{{Field: "recommended_directions", QuoteID: "s12", Values: []string{"data"}}, {Field: "weekly_workload", QuoteID: "s0"}, {Field: "recommended_goals", QuoteID: "s0", Values: []string{"try"}}, {Field: "prerequisites", QuoteID: "s2", Value: "none"}}
	raw, err := json.Marshal(aiVerificationClaims(claims))
	if err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	for i, c := range claims {
		want := "literal_fact"
		if i == 0 || i == 2 {
			want = "curriculum_interpretation"
		}
		if out[i]["claim_index"] != float64(i) || out[i]["evaluation_rule"] != want || out[i]["field"] != c.Field || out[i]["quote_id"] != c.QuoteID {
			t.Fatal(out)
		}
	}
	if len(out) != len(claims) {
		t.Fatal("review omitted a claim")
	}
}
