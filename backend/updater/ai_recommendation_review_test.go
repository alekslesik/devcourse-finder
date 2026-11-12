package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// Wire-contract tests do not simulate or claim real semantic model accuracy.
func TestAdvisoryReviewUsesIndependentRubricWithoutLiteralLane(t *testing.T) {
	for _, advisory := range []bool{true, false} {
		t.Run(map[bool]string{true: "advisory", false: "mixed"}[advisory], func(t *testing.T) {
			c, docs, x, _ := recommendationExtraction()
			calls := 0
			m := &DeepSeekModel{Key: "test", Model: "extractor", VerifyModel: "reviewer", recommendationsOnly: advisory}
			m.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				var body struct {
					Model    string `json:"model"`
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body.Model != "reviewer" {
					t.Fatal("extractor reviewed its own output", body.Model)
				}
				var input map[string]any
				if err := json.Unmarshal([]byte(body.Messages[1].Content), &input); err != nil {
					t.Fatal(err)
				}
				prompt := body.Messages[0].Content
				if advisory {
					if input["recommendations_only"] != true || !strings.HasPrefix(prompt, aiRecommendationVerificationPrompt) || !strings.Contains(prompt, aiRecommendationReviewOutputInstructions) || strings.Contains(prompt, "LANE literal_fact") {
						t.Fatal("mixed literal contract leaked into advisory review")
					}
					for _, d := range input["documents"].([]any) {
						if d.(map[string]any)["scope"] != "course" {
							t.Fatal("offer entered advisory review")
						}
					}
				} else if !strings.HasPrefix(prompt, aiVerificationPrompt) || input["recommendations_only"] == true {
					t.Fatal("literal mode lost its strict rubric")
				}
				out, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": `{"complete":true,"verified_claim_ids":[0]}`}}}})
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(out)))}, nil
			})}
			v, _, err := m.Verify(context.Background(), c, docs, x)
			if err != nil || !v.Complete || len(v.Verified) != 1 || v.Verified[0] != 0 {
				t.Fatal(v, err)
			}
			if advisory {
				x.Claims = append(x.Claims, AIClaim{Field: "duration", OfferID: "offer", DocumentID: "offer", Unit: "month"})
				if _, _, err = m.Verify(context.Background(), c, docs, x); err == nil || calls != 1 {
					t.Fatal("literal claim reached advisory model", err, calls)
				}
			}
		})
	}
}
