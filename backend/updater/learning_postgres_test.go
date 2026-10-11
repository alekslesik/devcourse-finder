package updater

import (
	"context"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestLearningQueuePublicationRoundTripAndRollback(t *testing.T) {
	db := postgresFixture(t)
	cfg := productionConfig(t)
	cfg.Sources = nil
	c := Candidate{Adapter: "otus", ExternalID: "python", URL: "https://otus.ru/lessons/python", FeedID: "otus-sitemap"}
	cfg.Discovery = []Feed{{ID: c.FeedID, Adapter: c.Adapter, URL: "https://otus.ru/sitemap.xml"}}
	s := Service{DB: db, Config: cfg, Worker: "publisher"}
	ctx := context.Background()
	now := time.Now().UTC()
	price := int64(1000000)
	facts := learningFixture(c, now)
	o := Observation{Price: &price, Enrollment: "open", Schedule: "scheduled", LearningObserved: true, LearningConditions: facts}
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
	if err != nil || len(loaded) != 1 || loaded[0].Offers[0].LearningConditions == nil || loaded[0].Offers[0].LearningOwnership == 0 || loaded[0].Offers[0].LearningConditions.WeeklyWorkload.Max != 12 {
		t.Fatal("round-trip failed", loaded, err)
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
	if !loaded[0].Offers[0].LearningConditions.Review.Evidence.CheckedAt.Equal(now) {
		t.Fatal("failure reconfirmed facts")
	}
	// Ack failure must roll back clearing facts in the same catalog transaction.
	_, err = db.Pool.Exec(ctx, `CREATE FUNCTION reject_learning_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'reject'; END $$; CREATE TRIGGER reject_learning_ack BEFORE UPDATE ON catalog_observations FOR EACH ROW WHEN (NEW.processed_at IS NOT NULL) EXECUTE FUNCTION reject_learning_ack()`)
	if err != nil {
		t.Fatal(err)
	}
	e.Observation.LearningConditions = nil
	e.Record.Offers[0].LearningConditions = nil
	e.ObservedAt = time.Now().UTC()
	if err = s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err == nil {
		t.Fatal("ack failure accepted")
	}
	loaded, _ = db.Load(ctx)
	if loaded[0].Offers[0].LearningConditions == nil {
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
	if loaded[0].Offers[0].LearningConditions != nil || loaded[0].Offers[0].LearningOwnership != 0 || loaded[0].Offers[0].Review {
		t.Fatal("successful missing evidence did not clear owned facts", loaded)
	}
}

func TestPracticumLearningFactsRemainTariffBound(t *testing.T) {
	db := postgresFixture(t)
	cfg := productionConfig(t)
	cfg.Sources = nil
	page, base, plus, c := sharedPracticum(t)
	cfg.Discovery = []Feed{{ID: c.FeedID, Adapter: c.Adapter, URL: practicumOrigin + "/lang-static/sitemap/"}}
	s := Service{DB: db, Config: cfg, Worker: "publisher"}
	ctx := context.Background()
	r, o, _, err := fetchPracticumGroup(ctx, sharedPracticumClient(t, base, plus, false), page, fingerprint(page), c)
	if err != nil {
		t.Fatal(err)
	}
	now := r.CheckedAt
	evidence := catalog.LearningEvidence{Source: practicumProfessionURL(c.ExternalID), Locator: "profession.support", Digest: strings.Repeat("a", 64), CheckedAt: now}
	baseFacts := &catalog.LearningConditions{Review: &catalog.LearningSupport{Available: true, Evidence: evidence}}
	evidence.Source = practicumProfessionURL("python-developer-plus")
	plusFacts := &catalog.LearningConditions{Mentor: &catalog.LearningSupport{Available: true, Evidence: evidence}}
	r.Offers[0].LearningConditions = baseFacts
	r.Offers[1].LearningConditions = plusFacts
	o.LearningByOffer = map[string]*catalog.LearningConditions{r.Offers[0].ID: baseFacts, r.Offers[1].ID: plusFacts}
	e := QueuedObservation{Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: now, Digest: fingerprint(page)}
	if err = s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ := db.Load(ctx)
	if len(loaded) != 1 || len(loaded[0].Offers) != 2 {
		t.Fatal(loaded)
	}
	for _, offer := range loaded[0].Offers {
		if offer.ID == r.Offers[0].ID && (offer.LearningConditions == nil || offer.LearningConditions.Review == nil || offer.LearningConditions.Mentor != nil || !offer.Review) {
			t.Fatal("base tariff lost review or borrowed mentor", offer)
		}
		if offer.ID == r.Offers[1].ID && (offer.LearningConditions == nil || offer.LearningConditions.Mentor == nil || offer.LearningConditions.Review != nil || !offer.Mentor) {
			t.Fatal("plus tariff borrowed review or lost mentor", offer)
		}
	}
	// Explicitly missing plus support clears only that tariff's owned facts.
	e.Record.Offers = append([]catalog.Offer(nil), r.Offers...)
	e.Record.Offers[0].LearningConditions = nil
	e.Record.Offers[1].LearningConditions = nil
	e.Observation.LearningByOffer = map[string]*catalog.LearningConditions{r.Offers[1].ID: nil}
	e.ObservedAt = time.Now().UTC()
	if err = s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = db.Load(ctx)
	for _, offer := range loaded[0].Offers {
		if offer.ID == r.Offers[0].ID && (offer.LearningConditions == nil || offer.LearningConditions.Review == nil) {
			t.Fatal("unobserved base facts erased")
		}
		if offer.ID == r.Offers[1].ID && (offer.LearningConditions != nil || offer.Mentor) {
			t.Fatal("missing plus facts retained")
		}
	}
}

func TestLearningEvidenceRefreshDoesNotBreakPriceConfirmation(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	now := time.Now().UTC()
	price := int64(2000000)
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	c := Candidate{URL: "https://otus.ru/lessons/python"}
	o := Observation{Price: &price, Enrollment: "open", LearningObserved: true, LearningConditions: learningFixture(c, now.Add(-8*time.Hour))}
	accepted, err := confirm(ctx, tx, "course", o, now.Add(-8*time.Hour))
	if err != nil || accepted {
		t.Fatal(accepted, err)
	}
	o.LearningConditions = learningFixture(c, now)
	o.LearningConditions.Review.Evidence.Digest = strings.Repeat("b", 64)
	accepted, err = confirm(ctx, tx, "course", o, now)
	if err != nil || !accepted {
		t.Fatal("fresh metadata prevented price confirmation", accepted, err)
	}
}

func TestPracticumDurationOwnershipUpgradeAndMissingPlus(t *testing.T) {
	db := postgresFixture(t)
	cfg := productionConfig(t)
	cfg.Sources = nil
	page, base, plus, c := sharedPracticum(t)
	cfg.Discovery = []Feed{{ID: c.FeedID, Adapter: c.Adapter, URL: practicumOrigin + "/lang-static/sitemap/"}}
	s := Service{DB: db, Config: cfg, Worker: "publisher"}
	ctx := context.Background()
	r, o, _, err := fetchPracticumGroup(ctx, sharedPracticumClient(t, base, plus, false), page, fingerprint(page), c)
	if err != nil {
		t.Fatal(err)
	}
	e := QueuedObservation{Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: r.CheckedAt, Digest: fingerprint(page)}
	if err = s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ := db.Load(ctx)
	for _, offer := range loaded[0].Offers {
		if !offer.DurationCollected {
			t.Fatal("newly appended tariff has no ownership", offer)
		}
	}
	// Simulate a v0.17.1 database without the private ownership column.
	if _, err = db.Pool.Exec(ctx, "ALTER TABLE offers DROP COLUMN duration_collected"); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = db.Load(ctx)
	for _, offer := range loaded[0].Offers {
		if !offer.DurationCollected {
			t.Fatal("prior collector ownership not recovered", offer)
		}
	}
	// A missing source duration on the first new-version read must clear both.
	e.Record.Offers = append([]catalog.Offer(nil), r.Offers...)
	for i := range e.Record.Offers {
		e.Record.Offers[i].DurationMonths = nil
	}
	e.Observation.DurationMonths = nil
	e.Observation.PracticumTariffs = append([]PracticumTariff(nil), o.PracticumTariffs...)
	for i := range e.Observation.PracticumTariffs {
		e.Observation.PracticumTariffs[i].DurationMonths = nil
	}
	e.ObservedAt = time.Now().UTC()
	if err = s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = db.Load(ctx)
	for _, offer := range loaded[0].Offers {
		if offer.DurationMonths != nil || offer.DurationCollected {
			t.Fatal("upgrade retained vanished duration", offer)
		}
	}
	// Later explicit operator duration is preserved, even across another upgrade.
	curated := loaded[0]
	curated.Offers = append([]catalog.Offer(nil), curated.Offers[:1]...)
	months := 6
	curated.Offers[0].DurationMonths = &months
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"practicum.yandex.ru"}, Courses: []catalog.Course{curated}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	e.ObservedAt = time.Now().UTC()
	if err = s.enqueue(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	// An automatic unchanged write after the operator import is not ownership proof.
	if _, err = db.Pool.Exec(ctx, "ALTER TABLE offers DROP COLUMN duration_collected"); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = db.Load(ctx)
	for _, offer := range loaded[0].Offers {
		if offer.ID == curated.Offers[0].ID && (offer.DurationMonths == nil || *offer.DurationMonths != 6 || offer.DurationCollected) {
			t.Fatal("curated duration reclassified by upgrade", offer)
		}
	}
	// Schema reruns must not overwrite ownership decisions after initial upgrade.
	if _, err = db.Pool.Exec(ctx, "UPDATE offers SET duration_collected=true WHERE id=$1", curated.Offers[0].ID); err != nil {
		t.Fatal(err)
	}
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var owned bool
	if err = db.Pool.QueryRow(ctx, "SELECT duration_collected FROM offers WHERE id=$1", curated.Offers[0].ID).Scan(&owned); err != nil || !owned {
		t.Fatal("ownership backfill reran", err)
	}
}
