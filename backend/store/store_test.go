package store

import (
	"context"
	"devcourse-finder/catalog"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostgresImportLifecycle(t *testing.T) {
	connection := os.Getenv("TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	parsed, err := url.Parse(connection)
	if err != nil || !strings.HasSuffix(parsed.Path, "_test") {
		t.Fatal("integration tests require a dedicated database ending in _test")
	}
	ctx := context.Background()
	db, err := Open(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Pool.Close()
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, "TRUNCATE courses,offers,imports,events,daily_stats,settings RESTART IDENTITY CASCADE"); err != nil {
		t.Fatal(err)
	}
	cached := NewCached(db)
	file, err := os.Open("../../data/demo-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	data, err := catalog.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Import(ctx, data, "integration-test", true); err != nil {
		t.Fatal(err)
	}
	loaded, err := cached.Load(ctx)
	if err != nil || len(loaded) != 0 {
		t.Fatalf("dry-run changed catalog: %v, %d", err, len(loaded))
	}
	for i := 0; i < 2; i++ {
		if err = db.Import(ctx, data, "integration-test", false); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err = cached.Load(ctx)
	if err != nil || len(loaded) != 20 {
		t.Fatalf("repeated import must preserve 20 unique programs: %v, %d", err, len(loaded))
	}
	var count int
	if err = db.Pool.QueryRow(ctx, "SELECT count(*) FROM offers").Scan(&count); err != nil || count != 20 {
		t.Fatalf("unexpected offers count: %d, %v", count, err)
	}
	previousSnapshot := loaded
	first := data.Courses[0]
	first.ID = "rollback-new"
	first.Slug = "rollback-new"
	first.Offers = nil
	second := data.Courses[1]
	second.ID = "rollback-conflict"
	second.Slug = data.Courses[0].Slug
	second.Offers = nil
	broken := catalog.Dataset{Courses: []catalog.Course{first, second}, Domains: data.Domains}
	if err = db.Import(ctx, broken, "integration-test", false); err == nil {
		t.Fatal("expected duplicate slug failure")
	}
	if err = db.Pool.QueryRow(ctx, "SELECT count(*) FROM courses WHERE id='rollback-new'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed import partially applied: %d, %v", count, err)
	}
	if err = db.Pool.QueryRow(ctx, "SELECT count(*) FROM imports").Scan(&count); err != nil || count != 2 {
		t.Fatalf("failed import persisted audit revision: %d, %v", count, err)
	}
	stable, err := cached.Load(ctx)
	if err != nil || &stable[0] != &previousSnapshot[0] {
		t.Fatal("rollback invalidated the committed snapshot", err)
	}
	data.Courses[0].Status = "draft"
	data.Courses[1].Status = "archived"
	if err = db.Import(ctx, data, "integration-test", false); err != nil {
		t.Fatal(err)
	}
	loaded, err = cached.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if previousSnapshot[0].Status != "published" {
		t.Fatal("refresh mutated a snapshot still in use")
	}
	cold := NewCached(db)
	var workers sync.WaitGroup
	for range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			current, err := cold.Load(ctx)
			if err != nil || len(current) != 20 || current[0].Status != "draft" {
				t.Errorf("concurrent cached load: %v", err)
			}
		}()
	}
	workers.Wait()
	found := catalog.Search(loaded, catalog.Filter{IncludeClosed: true}, time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC))
	if len(found) != 18 {
		t.Fatalf("draft and archive leaked into search: %d", len(found))
	}
}
