package updater

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func observationFixture(t *testing.T, s *Service, at time.Time) QueuedObservation {
	t.Helper()
	raw, err := os.ReadFile("testdata/stepik-go.json")
	if err != nil {
		t.Fatal(err)
	}
	candidate := Candidate{Adapter: "stepik", ExternalID: "54403", URL: "https://stepik.org/course/54403/promo", FeedID: "stepik"}
	record, o, err := collectCandidate(raw, candidate, at)
	if err != nil {
		t.Fatal(err)
	}
	return QueuedObservation{Version: 1, Kind: "discovered", Candidate: candidate, Record: record, Observation: o, ObservedAt: at.UTC().Truncate(time.Microsecond), Digest: fingerprint(raw)}
}
func observationService(t *testing.T) *Service {
	c := productionConfig(t)
	c.Discovery = []Feed{{ID: "stepik", Adapter: "stepik", URL: "https://stepik.org/sitemap.xml"}}
	return &Service{DB: postgresFixture(t), Config: c}
}
func TestObservationQueuePublishesAtomicallyAndDeduplicates(t *testing.T) {
	s := observationService(t)
	ctx := context.Background()
	e := observationFixture(t, s, time.Now().Add(-time.Minute))
	for i := 0; i < 2; i++ {
		if err := s.enqueue(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	old, err := s.DB.Load(ctx)
	if err != nil || len(old) != 0 {
		t.Fatal("enqueue published a course", old, err)
	}
	var pending int
	s.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_observations").Scan(&pending)
	if pending != 1 {
		t.Fatal("duplicate observation", pending)
	}
	result, err := s.Publish(ctx)
	if err != nil || result.Published != 1 {
		t.Fatal(result, err)
	}
	old, err = s.DB.Load(ctx)
	if err != nil || len(old) != 1 || old[0].Offers[0].PriceCheckedAt.Sub(e.ObservedAt) > time.Microsecond {
		t.Fatal("publication timestamp changed", old, err)
	}
	result, err = s.Publish(ctx)
	if err != nil || result.Published != 0 {
		t.Fatal("event replayed", result, err)
	}
	var imports int
	s.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM imports").Scan(&imports)
	if imports != 1 {
		t.Fatal("repeated audit", imports)
	}
	var done bool
	s.DB.Pool.QueryRow(ctx, "SELECT processed_at IS NOT NULL FROM catalog_observations").Scan(&done)
	if !done {
		t.Fatal("no acknowledgement")
	}
}
func TestPublicationRollbackLeavesObservationPending(t *testing.T) {
	s := observationService(t)
	ctx := context.Background()
	e := observationFixture(t, s, time.Now().Add(-time.Minute))
	if err := s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	_, err := s.DB.Pool.Exec(ctx, `CREATE FUNCTION reject_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'ack interrupted'; END $$; CREATE TRIGGER reject_ack BEFORE UPDATE ON catalog_observations FOR EACH ROW EXECUTE FUNCTION reject_ack()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err == nil {
		t.Fatal("ack failure ignored")
	}
	var imports, identities int
	var done bool
	s.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM imports").Scan(&imports)
	s.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_identities").Scan(&identities)
	s.DB.Pool.QueryRow(ctx, "SELECT processed_at IS NOT NULL FROM catalog_observations").Scan(&done)
	if imports != 0 || identities != 0 || done {
		t.Fatal("partial publication committed", imports, identities, done)
	}
	s.DB.Pool.Exec(ctx, "DROP TRIGGER reject_ack ON catalog_observations")
	if result, err := s.Publish(ctx); err != nil || result.Published != 1 {
		t.Fatal("retry did not recover", result, err)
	}
}
func TestPublisherRejectsPoisonAndOldEventsWithoutBlockingQueue(t *testing.T) {
	s := observationService(t)
	ctx := context.Background()
	e := observationFixture(t, s, time.Now().Add(-time.Minute))
	bad := e
	bad.Record = e.Record
	bad.Record.Source = "https://example.invalid/"
	raw, _ := json.Marshal(bad)
	_, err := s.DB.Pool.Exec(ctx, "INSERT INTO catalog_observations(canonical_url,observed_at,kind,payload) VALUES($1,$2,$3,$4)", bad.Candidate.URL, bad.ObservedAt, bad.Kind, raw)
	if err != nil {
		t.Fatal(err)
	}
	e.ObservedAt = e.ObservedAt.Add(time.Second)
	if err = s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	result, err := s.Publish(ctx)
	if err != nil || result.Failed != 1 || result.Published != 1 {
		t.Fatal(result, err)
	}
	older := e
	older.ObservedAt = e.ObservedAt.Add(-49 * time.Hour)
	if err = s.enqueue(ctx, older); err != nil {
		t.Fatal(err)
	}
	result, err = s.Publish(ctx)
	if err != nil || result.Published != 0 {
		t.Fatal("old observation published", result, err)
	}
	var code string
	s.DB.Pool.QueryRow(ctx, "SELECT code FROM catalog_observations ORDER BY id DESC LIMIT 1").Scan(&code)
	if code != "stale_observation" {
		t.Fatal(code)
	}
}
func TestQueuedFailureBreaksConfirmationAndDoesNotPublish(t *testing.T) {
	s := observationService(t)
	ctx := context.Background()
	e := observationFixture(t, s, time.Now().Add(-time.Minute))
	if err := s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	_, err := s.DB.Pool.Exec(ctx, "INSERT INTO updater_candidates VALUES($1,'fingerprint',now(),1)", e.Record.ID)
	if err != nil {
		t.Fatal(err)
	}
	failed := QueuedObservation{Kind: "failure", Candidate: e.Candidate, ObservedAt: time.Now(), FailureCode: "source_unavailable"}
	if err = s.enqueue(ctx, failed); err != nil {
		t.Fatal(err)
	}
	var count int
	s.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&count)
	if count != 1 {
		t.Fatal("collector changed confirmation")
	}
	result, err := s.Publish(ctx)
	if err != nil || result.Published != 0 {
		t.Fatal(result, err)
	}
	s.DB.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&count)
	if count != 0 {
		t.Fatal("failed intervening observation ignored")
	}
	old, _ := s.DB.Load(ctx)
	if len(old) != 1 {
		t.Fatal("failed read removed publication")
	}
}
func TestObservationContractRejectsInvalidIdentityAndErrorText(t *testing.T) {
	s := observationService(t)
	e := observationFixture(t, s, time.Now())
	e.Candidate.URL = "https://evil.invalid/course/54403/promo"
	if err := s.enqueue(context.Background(), e); err == nil {
		t.Fatal("bad identity queued")
	}
	e = observationFixture(t, s, time.Now())
	e.Kind = "failure"
	e.FailureCode = "secret response " + strings.Repeat("x", 100)
	if err := s.enqueue(context.Background(), e); err == nil {
		t.Fatal("raw error accepted")
	}
	e = observationFixture(t, s, time.Now())
	e.Record.Offers = append(e.Record.Offers, catalog.Offer{})
	if err := s.enqueue(context.Background(), e); err == nil {
		t.Fatal("ambiguous tariffs queued")
	}
}
