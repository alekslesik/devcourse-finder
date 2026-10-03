package catalog

import (
	"net/url"
	"testing"
)

func TestQueryBoundaries(t *testing.T) {
	for _, query := range []string{"language=rust", "experience=senior", "min=-1", "max=10000000001", "min=20&max=10", "hours=0", "hours=169", "page=0", "page=1000001", "page_size=0", "page_size=49", "include_free=1", "include_closed=yes", "sort=commission", "max=abc"} {
		t.Run(query, func(t *testing.T) {
			values, err := url.ParseQuery(query)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = Parse(values); err == nil {
				t.Fatal("invalid query accepted")
			}
		})
	}
	for _, query := range []string{"min=0&max=10000000000", "hours=168&page=1000000&page_size=48", "include_free=true&include_closed=false", "unknown=ignored"} {
		t.Run(query, func(t *testing.T) {
			values, err := url.ParseQuery(query)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = Parse(values); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestOutboundURLRestrictions(t *testing.T) {
	domains := []string{"example.com", "localhost", "127.0.0.1", "school.internal", "school.local"}
	for _, raw := range []string{"http://example.com", "javascript:alert(1)", "https://evil.example", "https://example.com.evil.test", "https://user:password@example.com", "https://127.0.0.1", "https://localhost", "https://school.internal", "https://school.local", "https://example.com:8443", "//example.com"} {
		t.Run(raw, func(t *testing.T) {
			if SafeURL(raw, domains) {
				t.Fatal("unsafe outbound URL accepted")
			}
		})
	}
	if !SafeURL("https://example.com/course?campaign=demo", domains) {
		t.Fatal("approved HTTPS URL rejected")
	}
}
