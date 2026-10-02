package main

import (
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
