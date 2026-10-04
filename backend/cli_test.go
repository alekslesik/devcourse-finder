package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

// Execute the actual production entry point in a separate OS process, including
// its JSON logger and os.Exit, without replacing run() or the repository.
func TestCLIChildProcess(t *testing.T) {
	if os.Getenv("DEVCOURSE_CLI_CHILD") != "1" {
		return
	}
	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		os.Exit(2)
	}
	os.Args = append([]string{os.Args[0]}, os.Args[separator+1:]...)
	main()
	os.Exit(0)
}

type cliResult struct {
	exit           int
	stdout, stderr string
	records        []map[string]any
}

func cliProcess(t *testing.T, connection string, extra map[string]string, args ...string) cliResult {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, append([]string{"-test.run=^TestCLIChildProcess$", "--"}, args...)...)
	variables := map[string]string{"DEVCOURSE_CLI_CHILD": "1", "DATABASE_URL": connection, "CATALOG_OPERATOR": "cli-test", "APP_ENV": "", "PGHOST": "", "PGPORT": "", "PGUSER": "", "PGPASSWORD": "", "PGDATABASE": "", "PGSSLMODE": "", "PGSERVICE": "", "PGSERVICEFILE": ""}
	for key, value := range extra {
		variables[key] = value
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, override := variables[key]; !override {
			command.Env = append(command.Env, entry)
		}
	}
	for key, value := range variables {
		command.Env = append(command.Env, key+"="+value)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if ctx.Err() != nil {
		t.Fatal("CLI process timed out")
	}
	result := cliResult{stdout: stdout.String(), stderr: stderr.String()}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatal(err)
		}
		result.exit = exit.ExitCode()
	}
	for _, line := range strings.Split(strings.TrimSpace(result.stderr), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("CLI stderr is not structured JSON: %v", err)
		}
		result.records = append(result.records, record)
	}
	return result
}
func assertCLIFailure(t *testing.T, result cliResult, command, stage, code string) map[string]any {
	t.Helper()
	if result.exit != 1 || len(result.records) != 1 {
		t.Fatalf("expected one error log and exit 1, got exit %d and %d records", result.exit, len(result.records))
	}
	record := result.records[0]
	if record["level"] != "ERROR" || record["msg"] != "command failed" || record["command"] != command || record["stage"] != stage || record["code"] != code {
		t.Fatalf("unexpected diagnostic: %v", record)
	}
	if id, ok := record["run_id"].(string); !ok || len(id) != 32 {
		t.Fatal("missing correlation run_id")
	}
	if message, ok := record["error"].(string); !ok || message == "" {
		t.Fatal("missing safe diagnostic message")
	}
	return record
}
func cliFile(t *testing.T, data catalog.Dataset) string {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCLISensitiveFailures(t *testing.T) {
	secret := "sentinel-private-value"
	rawURL := "https://example.com/private?token=" + secret
	source := httpFixture()
	invalid := catalog.Dataset{Courses: source.courses, Domains: source.domains}
	invalid.Courses[0].ID = rawURL
	invalidFile := cliFile(t, invalid)
	unknownFile := filepath.Join(t.TempDir(), "unknown.json")
	raw, _ := json.Marshal(map[string]any{rawURL: true})
	if err := os.WriteFile(unknownFile, raw, 0600); err != nil {
		t.Fatal(err)
	}
	syntaxFile := filepath.Join(t.TempDir(), "syntax.json")
	if err := os.WriteFile(syntaxFile, []byte(`{"courses": [!}`), 0600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, connection, stage, code, command string
		args                                   []string
		extra                                  map[string]string
	}{
		{name: "invalid DSN", connection: "postgres://user:" + secret + "@127.0.0.1:badport/devcourse_test", stage: "database_config", code: "invalid_database_configuration", command: "catalog_import", args: []string{"catalog", "import", invalidFile}},
		{name: "missing file", stage: "read_catalog", code: "catalog_file_unavailable", command: "catalog_validate", args: []string{"catalog", "validate", filepath.Join(t.TempDir(), secret, "missing.json")}},
		{name: "invalid course ID", stage: "validate_catalog", code: "invalid_catalog", command: "catalog_validate", args: []string{"catalog", "validate", invalidFile}},
		{name: "unknown JSON field", stage: "validate_catalog", code: "invalid_catalog", command: "catalog_validate", args: []string{"catalog", "validate", unknownFile}},
		{name: "JSON syntax", stage: "validate_catalog", code: "invalid_catalog", command: "catalog_validate", args: []string{"catalog", "validate", syntaxFile}},
		{name: "unknown command", stage: "arguments", code: "invalid_arguments", command: "unknown", args: []string{rawURL}},
		{name: "missing operator", stage: "operator", code: "invalid_catalog_operator", command: "catalog_import", args: []string{"catalog", "import", invalidFile}, extra: map[string]string{"CATALOG_OPERATOR": ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cliProcess(t, tt.connection, tt.extra, tt.args...)
			record := assertCLIFailure(t, result, tt.command, tt.stage, tt.code)
			if strings.Contains(result.stderr+result.stdout, secret) || strings.Contains(result.stderr+result.stdout, rawURL) {
				t.Fatal("CLI output disclosed a sentinel secret/URL")
			}
			if tt.name == "invalid course ID" && (record["course_index"] != float64(0) || record["validation_rule"] != "invalid or duplicate id/slug") {
				t.Fatal("missing safe validation location")
			}
			if tt.name == "JSON syntax" && record["json_offset"] == nil {
				t.Fatal("missing syntax offset")
			}
		})
	}
}

func TestCLIImportWithPostgres(t *testing.T) {
	db := postgresHTTPFixture(t)
	connection := db.Pool.Config().ConnString()
	ctx := context.Background()
	snapshot := func() string {
		t.Helper()
		courses, err := db.Load(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var revisions int
		if err = db.Pool.QueryRow(ctx, "SELECT count(*) FROM imports").Scan(&revisions); err != nil {
			t.Fatal(err)
		}
		var domains []byte
		if err = db.Pool.QueryRow(ctx, "SELECT value FROM settings WHERE key='domains'").Scan(&domains); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(struct {
			Courses   []catalog.Course
			Revisions int
			Domains   json.RawMessage
		}{courses, revisions, domains})
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	before := snapshot()
	source := httpFixture()
	first := source.courses[0]
	first.ID = "cli-new"
	first.Slug = "cli-new"
	first.Offers = append([]catalog.Offer(nil), first.Offers[:1]...)
	first.Offers[0].ID = "cli-new-offer"
	second := first
	second.ID = "cli-conflict"
	second.Slug = source.courses[0].Slug
	second.Offers = append([]catalog.Offer(nil), first.Offers...)
	second.Offers[0].ID = "cli-conflict-offer"
	file := cliFile(t, catalog.Dataset{Courses: []catalog.Course{first, second}, Domains: []string{"example.com", "extra.example.com"}})
	result := cliProcess(t, connection, nil, "catalog", "import", file)
	record := assertCLIFailure(t, result, "catalog_import", "publish_catalog", "catalog_import_failed")
	if record["sql_state"] != "23505" || record["constraint"] != "courses_slug_key" {
		t.Fatalf("missing safe SQL diagnostic: %v", record)
	}
	if snapshot() != before {
		t.Fatal("failed CLI import changed catalog, domains or committed revision")
	}
	if strings.Contains(result.stderr, "go-course") || strings.Contains(result.stderr, "Key (slug)") {
		t.Fatal("SQL diagnostic disclosed catalog values")
	}
	// A validated preview succeeds and changes neither the catalog nor audit data.
	dry := cliProcess(t, connection, nil, "catalog", "import", file, "--dry-run")
	if dry.exit != 0 || len(dry.records) != 1 || dry.records[0]["dry_run"] != true || dry.records[0]["msg"] != "catalog import completed" {
		t.Fatal("dry-run process did not report success")
	}
	if snapshot() != before {
		t.Fatal("CLI dry-run changed database")
	}
	// Validation errors also leave the already published catalog unchanged.
	invalid := catalog.Dataset{Courses: source.courses, Domains: source.domains}
	invalid.Courses[0].ID = "https://example.com/private?token=sentinel-private-value"
	failed := cliProcess(t, connection, nil, "catalog", "import", cliFile(t, invalid))
	assertCLIFailure(t, failed, "catalog_import", "validate_catalog", "invalid_catalog")
	if strings.Contains(failed.stderr, "sentinel-private-value") || snapshot() != before {
		t.Fatal("invalid import disclosed data or changed the database")
	}
	// A closed local endpoint exercises a real connection error without exposing
	// a DSN password or full URI in the diagnostic.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	offline, err := url.Parse(connection)
	if err != nil {
		t.Fatal("invalid fixture connection URI")
	}
	offline.Host = address
	offline.User = url.UserPassword(db.Pool.Config().ConnConfig.User, "sentinel-private-value")
	query := offline.Query()
	query.Set("connect_timeout", "1")
	offline.RawQuery = query.Encode()
	unavailable := cliProcess(t, offline.String(), nil, "catalog", "import", file)
	diagnostic := assertCLIFailure(t, unavailable, "catalog_import", "publish_catalog", "catalog_import_failed")
	if diagnostic["failure_kind"] != "database_connection" {
		t.Fatal("connection failure is not identifiable")
	}
	if strings.Contains(unavailable.stderr+unavailable.stdout, "sentinel-private-value") || strings.Contains(unavailable.stderr, offline.String()) || snapshot() != before {
		t.Fatal("connection failure disclosed credentials or changed data")
	}
	// Successful publication commits the update and records a structured success.
	updated := httpFixture()
	updated.courses[0].Offers[0].Price = value(int64(7654321))
	success := cliProcess(t, connection, nil, "catalog", "import", cliFile(t, catalog.Dataset{Courses: updated.courses, Domains: updated.domains}))
	if success.exit != 0 || len(success.records) != 1 || success.records[0]["dry_run"] != false || success.records[0]["courses"] != float64(3) {
		t.Fatal("successful CLI import did not report completion")
	}
	var price int64
	if err := db.Pool.QueryRow(ctx, "SELECT price FROM offers WHERE id='self'").Scan(&price); err != nil || price != 7654321 {
		t.Fatalf("CLI price update missing: %v", err)
	}
}
