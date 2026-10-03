package main

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCatalogOperator(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "trimmed", raw: "  operator-login  ", want: "operator-login"},
		{name: "unicode", raw: "оператор", want: "оператор"},
		{name: "empty", wantErr: true},
		{name: "whitespace", raw: " \t\n", wantErr: true},
		{name: "too long", raw: strings.Repeat("x", 101), wantErr: true},
		{name: "invalid UTF-8", raw: string([]byte{0xff}), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := catalogOperator(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("catalogOperator() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("catalogOperator() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFailureResponseContract(t *testing.T) {
	for status, code := range map[int]string{400: "invalid_request", 404: "not_found", 429: "rate_limited", 503: "unavailable", 500: "internal_error"} {
		t.Run(code, func(t *testing.T) {
			response := httptest.NewRecorder()
			response.Header().Set("X-Request-ID", "test-request")
			fail(response, status, "Safe message")
			if response.Code != status {
				t.Fatalf("status = %d", response.Code)
			}
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["code"] != code || body["request_id"] != "test-request" || body["error"] != "Safe message" {
				t.Fatalf("unexpected error envelope: %v", body)
			}
			if response.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("error must not be cached")
			}
		})
	}
}
