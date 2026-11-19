package main

import "testing"

func TestAccountHTTPDisabledAndInvalidConfiguration(t *testing.T) {
	for _, flag := range []string{"", "false"} {
		t.Setenv("AUTH_ACTIONS_ENABLED", flag)
		handler, err := accountHTTP(nil)
		if err != nil || handler != nil {
			t.Fatal("disabled actions mounted")
		}
	}
	for _, flag := range []string{"TRUE", "1", "true"} {
		t.Setenv("AUTH_ACTIONS_ENABLED", flag)
		t.Setenv("AUTH_GATEWAY_SECRET", "")
		handler, err := accountHTTP(nil)
		if err == nil || handler != nil {
			t.Fatal("invalid configuration did not fail closed")
		}
	}
}
