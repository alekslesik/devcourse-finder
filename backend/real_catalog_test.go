package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
)

func TestRealCatalogCLIImportAndPublicHTTP(t *testing.T) {
	db := postgresHTTPFixture(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, "TRUNCATE courses CASCADE"); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open("../data/real-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := catalog.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	path := cliFile(t, data)
	connection := db.Pool.Config().ConnString()
	var before int
	if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM imports").Scan(&before); err != nil {
		t.Fatal(err)
	}
	preview := cliProcess(t, connection, nil, "catalog", "import", path, "--dry-run")
	if preview.exit != 0 {
		t.Fatal("real catalog preview failed")
	}
	var count, revisions int
	if err := db.Pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM courses),(SELECT count(*) FROM imports)").Scan(&count, &revisions); err != nil || count != 0 || revisions != before {
		t.Fatal("dry-run changed catalog or audit")
	}
	var previous string
	for i := 0; i < 2; i++ {
		result := cliProcess(t, connection, nil, "catalog", "import", path)
		if result.exit != 0 {
			t.Fatal("real catalog CLI import failed")
		}
		loaded, err := db.Load(ctx)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(loaded)
		if len(loaded) != 20 || (i > 0 && previous != string(raw)) {
			t.Fatal("real catalog changed on repeated import")
		}
		previous = string(raw)
	}
	if err := db.Pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM courses),(SELECT count(*) FROM offers)").Scan(&count, &revisions); err != nil || count != 20 || revisions != 24 {
		t.Fatalf("duplicate/missing courses or offers: %d/%d %v", count, revisions, err)
	}
	server := httptest.NewServer(newHandler(store.NewCached(db)))
	defer server.Close()
	client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	contract := newResponseContract(t)
	for _, language := range []string{"go", "python", "java", "javascript"} {
		body := contract.check(t, wireRequest(t, client, server.URL, "GET", "/api/v1/courses?language="+language+"&include_closed=true&page_size=48", ""), "/api/v1/courses", "GET", 200).(map[string]any)
		if body["total"] != float64(5) {
			t.Fatalf("missing %s programs after import", language)
		}
	}
	for _, course := range data.Courses {
		detail := contract.check(t, wireRequest(t, client, server.URL, "GET", "/api/v1/courses/"+course.Slug, ""), "/api/v1/courses/{slug}", "GET", 200).(map[string]any)
		if detail["course"].(map[string]any)["demo"] != false {
			t.Fatal("real course displayed as demo")
		}
	}
	comparison := contract.check(t, wireRequest(t, client, server.URL, "GET", "/api/v1/compare?offer_ids=hexlet-java-standard,hexlet-java-premium,codebasics-java-course", ""), "/api/v1/compare", "GET", 200).([]any)
	if len(comparison) != 3 || comparison[0].(map[string]any)["offer"].(map[string]any)["price"] != float64(8500000) || comparison[1].(map[string]any)["offer"].(map[string]any)["mentor"] != true {
		t.Fatal("full price or premium support lost in comparison")
	}
	response := wireRequest(t, client, server.URL, "GET", "/out/hexlet-java-standard?url=https://attacker.example", "")
	contract.check(t, response, "/out/{offer_id}", "GET", 302)
	if response.Header().Get("Location") != "https://ru.hexlet.io/programs/java" {
		t.Fatal("real official redirect did not match reviewed source")
	}
}
