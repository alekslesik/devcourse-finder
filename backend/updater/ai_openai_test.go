package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestOpenAIStructuredOutputAndFixedDestination(t *testing.T) {
	c, docs, extraction, _ := aiFixture()
	calls := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.openai.com/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Fatal("wrong endpoint or authorization")
		}
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			t.Fatal("invalid request")
		}
		if request["store"] != false || request["response_format"].(map[string]any)["type"] != "json_schema" {
			t.Fatal(request)
		}
		payload, _ := json.Marshal(extraction)
		if calls == 2 {
			payload = []byte(`{"complete":true,"verified_claim_ids":[0,1]}`)
		}
		response, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(payload), "refusal": nil}}}, "usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 50, "total_tokens": 150}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(response)))}, nil
	})}
	model := &OpenAIModel{Key: "test-secret", Model: "gpt-4.1-mini", Client: client}
	x, usage, err := model.Extract(context.Background(), c, docs)
	if err != nil || usage.Total != 150 {
		t.Fatal(usage, err)
	}
	v, _, err := model.Verify(context.Background(), c, docs, x)
	if err != nil || calls != 2 || !v.Complete {
		t.Fatal(v, err)
	}
	if strings.Contains(fmt.Sprint(model), "test-secret") {
		t.Fatal("credential formatted")
	}
}
func TestOpenAIRejectsFailureRefusalAndTruncation(t *testing.T) {
	c, docs, _, _ := aiFixture()
	for _, body := range []string{`{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`, `{"choices":[{"finish_reason":"stop","message":{"content":"{}","refusal":"No"}}]}`, `{"choices":[]}`, `not json`} {
		model := &OpenAIModel{Key: "test-secret", Model: "gpt-4.1-mini", Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}}
		if _, _, err := model.Extract(context.Background(), c, docs); err == nil {
			t.Fatal("invalid model response accepted")
		}
	}
}

func TestOpenAISeparateReviewerAndSafeQuotaClassification(t *testing.T) {
	c, docs, x, _ := aiFixture()
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		var req map[string]any
		json.NewDecoder(r.Body).Decode(&req)
		if req["model"] != "gpt-4.1" {
			t.Fatal("reviewer reused extractor")
		}
		return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"insufficient_quota","message":"a response can contain a secret; never log this"}}`))}, nil
	})}
	model := &OpenAIModel{Key: "test-secret", Model: "gpt-4.1-mini", VerifyModel: "gpt-4.1", Client: client}
	_, _, err := model.Verify(context.Background(), c, docs, x)
	if err == nil || err.Error() != "openai_insufficient_quota" {
		t.Fatal("unsafe provider error", err)
	}
}
