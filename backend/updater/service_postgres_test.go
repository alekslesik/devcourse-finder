package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
)

func postgresFixture(t *testing.T) *store.DB {
	t.Helper()
	raw := os.Getenv("TEST_DATABASE_URL")
	if raw == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.HasSuffix(u.Path, "_test") {
		t.Fatal("requires dedicated _test database")
	}
	ctx := context.Background()
	admin, err := store.Open(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	name := fmt.Sprintf("updater_test_%d", time.Now().UnixNano())
	schema := pgx.Identifier{name}.Sanitize()
	if _, err = admin.Pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Pool.Close()
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", name)
	u.RawQuery = q.Encode()
	db, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Pool.Close()
		admin.Pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Pool.Close()
	})
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}
func productionConfig(t *testing.T) Config {
	t.Helper()
	c, err := LoadConfig("../../data/updater-sources.json", "../../data/real-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	return c
}
func clientResponse(status int, body string) *http.Client {
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
}
func TestRunPublishesVerifiedSourceAndPreservesDataOnFailure(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	ctx := context.Background()
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		status := 200
		body := string(stepikJSON(validStepik()))
		if strings.HasSuffix(r.URL.Path, "58852") {
			status = 404
			body = "not found"
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
	})}
	service := Service{DB: db, Config: config, Client: client}
	result, err := service.Run(ctx)
	if err != nil || result.Published != 1 || result.Failed != 1 || result.Status != "partial" {
		t.Fatalf("run: %#v %v", result, err)
	}
	old, err := db.Load(ctx)
	if err != nil || len(old) != 1 || old[0].ID != "stepik-golang" {
		t.Fatalf("unverified templates published: %#v %v", old, err)
	}
	cache := store.NewCached(db)
	beforeCache, err := cache.Load(ctx)
	if err != nil || len(beforeCache) != 1 {
		t.Fatal(err)
	}
	var before json.RawMessage
	if err = db.Pool.QueryRow(ctx, "SELECT before_data FROM imports ORDER BY id LIMIT 1").Scan(&before); err != nil || string(before) != "[]" {
		t.Fatalf("bad initial snapshot %s %v", before, err)
	}
	service.Client = clientResponse(404, "gone")
	for i := 0; i < 3; i++ {
		result, err = service.Run(ctx)
		if err != nil || result.Published != 0 || result.Status != "failed" {
			t.Fatalf("failed run published %#v %v", result, err)
		}
	}
	after, _ := db.Load(ctx)
	a, _ := json.Marshal(old)
	b, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("failed fetch changed previous data or freshness")
	}
	var imports, failures int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM imports").Scan(&imports)
	db.Pool.QueryRow(ctx, "SELECT failures FROM updater_sources WHERE source_id='stepik-golang'").Scan(&failures)
	if imports != 1 || failures != 3 {
		t.Fatalf("audit/monitoring mismatch %d/%d", imports, failures)
	}
	service.Client = client
	if _, err = service.Run(ctx); err != nil {
		t.Fatal(err)
	}
	refreshed, err := cache.Load(ctx)
	if err != nil || !refreshed[0].CheckedAt.After(beforeCache[0].CheckedAt) {
		t.Fatal("API catalog cache did not refresh after automatic import")
	}
}
func TestMergeKeepsOtherOffersManualFieldsAndAuditSnapshot(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	ctx := context.Background()
	course := config.Template("stepik-golang")
	course.Summary = "Operator description"
	extra := course.Offers[0]
	extra.ID = "operator-extra-offer"
	extra.Enrollment = "open"
	extra.URL = "https://operator-school.example/course"
	course.Offers = append(course.Offers, extra)
	course.Offers[0].PriceCheckedAt = time.Now().Add(-48 * time.Hour)
	if err := db.Import(ctx, catalog.Dataset{Domains: append(append([]string{}, config.Templates.Domains...), "operator-school.example"), Courses: []catalog.Course{course}}, "test", false); err != nil {
		t.Fatal(err)
	}
	old, _ := db.Load(ctx)
	service := Service{DB: db, Config: Config{Sources: config.Sources[:1], Templates: config.Templates}, Client: clientResponse(200, string(stepikJSON(validStepik())))}
	if _, err := service.Run(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := db.Load(ctx)
	if after[0].Summary != "Operator description" || len(after[0].Offers) != 2 {
		t.Fatal("manual fields or other tariff overwritten")
	}
	for _, o := range after[0].Offers {
		if o.ID == extra.ID && o.Enrollment != "open" {
			t.Fatal("omitted tariff closed")
		}
	}
	var snapshot []catalog.Course
	var raw []byte
	if err := db.Pool.QueryRow(ctx, "SELECT before_data FROM imports ORDER BY id DESC LIMIT 1").Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if json.Unmarshal(raw, &snapshot) != nil {
		t.Fatal("invalid before snapshot")
	}
	a, _ := json.Marshal(old)
	b, _ := json.Marshal(snapshot)
	if string(a) != string(b) {
		t.Fatal("audit before snapshot was mutated by merge")
	}
	// Explicit paid=true without a verified RUB price removes the old free label
	// without falsely extending the old price-check timestamp.
	c := validStepik()
	c["is_paid"] = true
	service.Client = clientResponse(200, string(stepikJSON(c)))
	old, _ = db.Load(ctx)
	if _, err := service.Run(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ = db.Load(ctx)
	for _, o := range after[0].Offers {
		if o.ID == course.Offers[0].ID {
			if o.Free || o.Price != nil || o.PriceKind != "unknown" {
				t.Fatal("unverified paid price advertised as free")
			}
			for _, prior := range old[0].Offers {
				if prior.ID == o.ID && !o.PriceCheckedAt.Equal(prior.PriceCheckedAt) {
					t.Fatal("missing price refreshed timestamp")
				}
			}
		}
	}
}
func TestConfirmationNeedsSeparateWindowAndFailureBreaksIt(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	ctx := context.Background()
	course := config.Template("stepik-golang")
	paid := int64(1000000)
	course.Offers[0].Price = &paid
	course.Offers[0].Free = false
	course.Offers[0].PriceCheckedAt = time.Now()
	if err := db.Import(ctx, catalog.Dataset{Domains: config.Templates.Domains, Courses: []catalog.Course{course}}, "test", false); err != nil {
		t.Fatal(err)
	}
	service := Service{DB: db, Config: Config{Sources: config.Sources[:1], Templates: config.Templates}, Client: clientResponse(200, string(stepikJSON(validStepik())))}
	for i := 0; i < 2; i++ {
		r, err := service.Run(ctx)
		if err != nil || r.Published != 0 {
			t.Fatal("immediate repeat confirmed anomalous price", err)
		}
	}
	service.Client = clientResponse(404, "")
	if _, err := service.Run(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&count)
	if count != 0 {
		t.Fatal("failed source retained confirmation")
	}
	service.Client = clientResponse(200, string(stepikJSON(validStepik())))
	if _, err := service.Run(ctx); err != nil {
		t.Fatal(err)
	}
	db.Pool.Exec(ctx, "UPDATE updater_candidates SET observed_at=now()-interval '12 hours'")
	r, err := service.Run(ctx)
	if err != nil || r.Published != 1 {
		t.Fatalf("second scheduled observation not published: %#v %v", r, err)
	}
	after, _ := db.Load(ctx)
	if !after[0].Offers[0].Free || *after[0].Offers[0].Price != 0 {
		t.Fatal("confirmed price not stored")
	}
}
func TestPublicationRollsBackAndReadsStateAfterManualLock(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	ctx := context.Background()
	course := config.Template("stepik-golang")
	if err := db.Import(ctx, catalog.Dataset{Domains: config.Templates.Domains, Courses: []catalog.Course{course}}, "test", false); err != nil {
		t.Fatal(err)
	}
	_, err := db.UpdateVerified(ctx, config.Templates.Domains, "automatic-catalog-updater", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
		tx.Exec(ctx, "INSERT INTO updater_candidates VALUES('rollback-test','hash',now(),1)")
		old[0].Offers[0].PriceKind = "invalid"
		return old, nil
	})
	if err == nil {
		t.Fatal("invalid publication accepted")
	}
	var candidates, imports int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&candidates)
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM imports").Scan(&imports)
	if candidates != 0 || imports != 1 {
		t.Fatal("failed transaction left data/history behind")
	}
	manual, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer manual.Rollback(ctx)
	if _, err = manual.Exec(ctx, "SELECT pg_advisory_xact_lock(829104)"); err != nil {
		t.Fatal(err)
	}
	manual.Exec(ctx, "UPDATE courses SET summary='Concurrent operator edit',status='archived' WHERE id='stepik-golang'")
	done := make(chan error, 1)
	go func() {
		_, err := db.UpdateVerified(ctx, config.Templates.Domains, "automatic-catalog-updater", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
			code := ""
			zero := int64(0)
			if old[0].Summary != "Concurrent operator edit" {
				return nil, fmt.Errorf("loaded a stale catalog before publication lock")
			}
			courses, err := merge(ctx, tx, old, course, course.ID, Observation{Price: &zero, Enrollment: "continuous"}, time.Now(), &code)
			if code != "protected_course" || len(courses) != 0 {
				return nil, fmt.Errorf("archived course revived")
			}
			return courses, err
		})
		done <- err
	}()
	if err = manual.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publication blocked after manual commit")
	}
}
func TestWorkerHealthAndOverlappingCollection(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	if Health(ctx, db) == nil {
		t.Fatal("missing heartbeat reported healthy")
	}
	if heartbeat(ctx, db) != nil || Health(ctx, db) != nil {
		t.Fatal("fresh heartbeat unhealthy")
	}
	db.Pool.Exec(ctx, `UPDATE settings SET value=to_jsonb(now()-interval '2 minutes') WHERE key='updater_heartbeat'`)
	if Health(ctx, db) == nil {
		t.Fatal("stale heartbeat reported healthy")
	}
	lock, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	lock.Exec(ctx, "SELECT pg_advisory_xact_lock(829105)")
	service := Service{DB: db, Config: config}
	if _, err = service.Run(ctx); err != ErrBusy {
		t.Fatalf("overlap not rejected: %v", err)
	}
}

func TestSchedulerInitialCollectionAndRestartDeduplication(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	config.Sources = config.Sources[:1]
	service := Service{DB: db, Config: config, Client: clientResponse(200, string(stepikJSON(validStepik())))}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Serve(ctx) }()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	ready := false
	for !ready {
		select {
		case <-deadline.C:
			cancel()
			t.Fatal("initial collection did not finish")
		case <-ticker.C:
			var count int
			if err := db.Pool.QueryRow(context.Background(), "SELECT count(*) FROM updater_runs WHERE finished_at IS NOT NULL").Scan(&count); err != nil {
				cancel()
				t.Fatal(err)
			}
			ready = count == 1
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	restart, stop := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer stop()
	if err := service.Serve(restart); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.Pool.QueryRow(context.Background(), "SELECT count(*) FROM updater_runs").Scan(&count); err != nil || count != 1 {
		t.Fatalf("restart duplicated schedule window: %d %v", count, err)
	}
	var output strings.Builder
	if err := Status(context.Background(), db, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `"status":"completed"`) || !strings.Contains(output.String(), `"code":"verified"`) {
		t.Fatal("missing run/source diagnostics")
	}
}
