package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPasswordCommandsRunWithoutDatabaseAndNeverReplaceKeys(t *testing.T) {
	t.Setenv("DATABASE_URL", "invalid")
	t.Setenv("APP_ENV", "production")
	original := os.Args
	defer func() { os.Args = original }()
	path := filepath.Join(t.TempDir(), "password-keyring.json")
	os.Args = []string{"devcourse-finder", "password-keyring", "init", path, "current"}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	os.Args = []string{"devcourse-finder", "password-keyring", "check", path}
	if err := run(); err != nil {
		t.Fatal(err)
	}
	os.Args = []string{"devcourse-finder", "password-keyring", "init", path, "current"}
	if err := run(); err == nil {
		t.Fatal("overwrote a keyring")
	}
	os.WriteFile(path, []byte(`{"private":"PRIVATE_SENTINEL"}`), 0600)
	os.Args = []string{"devcourse-finder", "password-keyring", "check", path}
	err := run()
	if err == nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") || strings.Contains(err.Error(), path) {
		t.Fatal("unsafe diagnostics")
	}
}

func TestEnabledPasswordStorageFailsBeforeDatabaseStartup(t *testing.T) {
	original := os.Args
	defer func() { os.Args = original }()
	os.Args = []string{"devcourse-finder"}
	t.Setenv("PASSWORD_STORAGE_ENABLED", "true")
	t.Setenv("PASSWORD_KEYRING_FILE", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("DATABASE_URL", "invalid")
	err := run()
	if err == nil || !strings.Contains(err.Error(), "Password storage configuration") {
		t.Fatal("missing key did not fail startup preflight")
	}
}
