package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestDeepSeekJSONModeFixedEndpointAndSeparateVerifier(t *testing.T) {
	c, docs, extraction, _ := aiFixture()
	calls := 0
	model := &DeepSeekModel{Key: "deepseek-test-secret", Model: "deepseek-flash", VerifyModel: "deepseek-v4-pro"}
	model.Client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://api.deepseek.com/chat/completions" || r.Header.Get("Authorization") != "Bearer deepseek-test-secret" { t.Fatal("wrong provider or key") }
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil { t.Fatal(err) }
		wantModel := model.Model
		if calls == 2 { wantModel = model.VerifyModel }
		if request["model"] != wantModel || request["max_tokens"] != float64(4096) || request["max_completion_tokens"] != nil || request["store"] != nil || request["response_format"].(map[string]any)["type"] != "json_object" { t.Fatal("wrong DeepSeek protocol", request["model"]) }
		if request["thinking"].(map[string]any)["type"] != "disabled" { t.Fatal("unbounded reasoning enabled") }
		messages := request["messages"].([]any)
		prompt := messages[0].(map[string]any)["content"].(string)
		if !strings.Contains(prompt, "Output JSON schema:") || !strings.Contains(prompt, "Shape example") { t.Fatal("JSON schema/example omitted") }
		payload, _ := json.Marshal(extraction)
		if calls == 2 { payload = []byte(`{"complete":true,"verified_claim_ids":[0,1]}`) }
		body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]any{"content": string(payload)}}}, "usage": AIUsage{Prompt: 100, Completion: 50, Total: 150}})
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	x, usage, err := model.Extract(context.Background(), c, docs)
	if err != nil || usage.Total != 150 { t.Fatal(usage, err) }
	v, _, err := model.Verify(context.Background(), c, docs, x)
	if err != nil || !v.Complete || calls != 2 { t.Fatal(v, err) }
	if strings.Contains(fmt.Sprint(model), model.Key) { t.Fatal("key leaked by formatting") }
}

func TestAIProviderErrorsAndNoCredentialCrossing(t *testing.T) {
	c, docs, _, _ := aiFixture()
	for _, test := range []struct { status int; code string }{{402,"deepseek_insufficient_quota"}, {401,"deepseek_authentication_failed"}, {429,"deepseek_rate_limited"}, {503,"deepseek_unavailable"}, {422,"deepseek_protocol_rejected"}} {
		model := &DeepSeekModel{Key: "deep-only", Model: "deepseek-flash", Client: &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Host != "api.deepseek.com" || r.Header.Get("Authorization") != "Bearer deep-only" { t.Fatal("credential crossed providers") }
			return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"private-body"}}`))}, nil
		})}}
		_, _, err := model.Extract(context.Background(), c, docs)
		if err == nil || err.Error() != test.code { t.Fatal(err, test.code) }
	}
	m := &OpenAIModel{Key: "open-only", Model: "gpt-4.1-mini", Client: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response,error) { return nil, errors.New("transport error with private details") })}}
	if _, _, err := m.Extract(context.Background(), c, docs); err == nil || err.Error() != "openai_unavailable" { t.Fatal(err) }
}

func TestDeepSeekStillRejectsMalformedRefusedAndUngroundedOutput(t *testing.T) {
	c, docs, _, _ := aiFixture()
	for _, body := range []string{`{"choices":[{"finish_reason":"length","message":{"content":"{}"}}]}`, `{"choices":[{"finish_reason":"stop","message":{"content":"{}","refusal":"No"}}]}`, `{"choices":[{"finish_reason":"stop","message":{"content":""}}]}`, `{"choices":[]}`} {
		m := &DeepSeekModel{Key: "test", Model: "deepseek-flash", Client: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response,error) {
			return &http.Response{StatusCode:200, Body:io.NopCloser(strings.NewReader(body))},nil
		})}}
		if _, _, err := m.Extract(context.Background(), c, docs); err == nil { t.Fatal("unsafe output accepted") }
	}
}
