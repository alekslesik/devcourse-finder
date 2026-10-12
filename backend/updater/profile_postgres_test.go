package updater

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestProfileQueueBackfillRoundTripAndRollback(t *testing.T) {
	db := postgresFixture(t)
	cfg := productionConfig(t)
	cfg.Sources = nil
	c := Candidate{Adapter: "otus", ExternalID: "python", URL: "https://otus.ru/lessons/python", FeedID: "otus-sitemap"}
	cfg.Discovery = []Feed{{ID: c.FeedID, Adapter: c.Adapter, URL: "https://otus.ru/sitemap.xml"}}
	s := Service{DB: db, Config: cfg, Worker: "publisher"}
	ctx := context.Background()
	now := time.Now().UTC()
	price := int64(1000000)
	// Backfill an existing published identity without changing its ID or offer.
	legacy, err := normalizedRecord(c, "Python-разработчик", "Программа по Python", "OTUS", Observation{Price: &price, Enrollment: "open", Schedule: "scheduled"}, now.Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"otus.ru"}, Courses: []catalog.Course{legacy}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	facts := profileFixture(c, now)
	o := Observation{Price: &price, Enrollment: "open", Schedule: "scheduled", ProfileObserved: true, Profile: facts}
	r, err := normalizedRecord(c, "Python-разработчик", "Программа по Python", "OTUS", o, now)
	if err != nil {
		t.Fatal(err)
	}
	e := QueuedObservation{Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: now, Digest: strings.Repeat("a", 64)}
	if err = s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.Load(ctx)
	if err != nil || len(loaded) != 1 || loaded[0].Profile == nil || loaded[0].ProfileOwnership == 0 || loaded[0].Audience[0] != "none" {
		t.Fatal("round-trip failed", loaded, err)
	}
	var status bytes.Buffer
	if err = profileStatus(ctx, db, &status); err != nil || !strings.Contains(status.String(), `"audience":1`) {
		t.Fatal(status.String(), err)
	}
	// Failed sources are separate events and cannot clear or reconfirm facts.
	failure := QueuedObservation{Kind: "failure", Candidate: c, ObservedAt: time.Now().UTC(), FailureCode: "source_unavailable"}
	if err = s.enqueue(ctx, failure); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = db.Load(ctx)
	if !loaded[0].Profile.Audience.Evidence.CheckedAt.Equal(now) {
		t.Fatal("failure reconfirmed facts")
	}
	// Ack failure must roll back clearing facts in the same catalog transaction.
	_, err = db.Pool.Exec(ctx, `CREATE FUNCTION reject_learning_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'reject'; END $$; CREATE TRIGGER reject_learning_ack BEFORE UPDATE ON catalog_observations FOR EACH ROW WHEN (NEW.processed_at IS NOT NULL) EXECUTE FUNCTION reject_learning_ack()`)
	if err != nil {
		t.Fatal(err)
	}
	e.Observation.Profile = nil
	e.Record.Profile = nil
	e.ObservedAt = time.Now().UTC()
	if err = s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err == nil {
		t.Fatal("ack failure accepted")
	}
	loaded, _ = db.Load(ctx)
	if loaded[0].Profile == nil {
		t.Fatal("rolled-back clear persisted")
	}
	_, err = db.Pool.Exec(ctx, "DROP TRIGGER reject_learning_ack ON catalog_observations; DROP FUNCTION reject_learning_ack()")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = db.Load(ctx)
	if loaded[0].Profile != nil || loaded[0].ProfileOwnership != 0 || loaded[0].Audience[0] != "unknown" {
		t.Fatal("successful missing evidence did not clear owned facts", loaded)
	}
}
