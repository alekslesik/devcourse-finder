package updater

import (
	"context"
	"devcourse-finder/catalog"
	"errors"
	"fmt"
	"testing"
)

func TestScheduleRecheckPreservesProtectedCandidatesAndWorkerLock(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	for _, state := range []string{"published", "protected", "pending"} {
		_, err := db.Pool.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state,attempted_at,next_attempt_at) VALUES('purpleschool',$1,$2,'purple',$1,now(),now()+interval '1 day')`, state, "https://purpleschool.ru/course/"+state)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"protected", "pending", "missing", "a';DROP TABLE courses;--"} {
		if err := ScheduleRecheck(ctx, db, "purpleschool", id); err == nil {
			t.Fatal("unsafe recheck", id)
		}
	}
	lock, err := db.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Rollback(ctx)
	if _, err = lock.Exec(ctx, "SELECT pg_advisory_xact_lock_shared(829105)"); err != nil {
		t.Fatal(err)
	}
	if err = ScheduleRecheck(ctx, db, "purpleschool", "published"); !errors.Is(err, ErrBusy) {
		t.Fatal("active worker not respected", err)
	}
	lock.Rollback(ctx)
	// Older due candidates fill more than the six-entry published lane. A recent
	// target must still be processed first by the immediately following run.
	for i := 0; i < providerLaneLimit+3; i++ {
		id := fmt.Sprintf("older-%d", i)
		if _, err = db.Pool.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state,attempted_at,next_attempt_at) VALUES('purpleschool',$1,$2,'purple','published',now()-interval '1 day',now()-interval '1 hour')`, id, "https://purpleschool.ru/course/"+id); err != nil {
			t.Fatal(err)
		}
	}
	if err = ScheduleRecheck(ctx, db, "purpleschool", "published"); err != nil {
		t.Fatal(err)
	}
	service := Service{DB: db, Config: Config{Discovery: []Feed{{Adapter: "purpleschool"}}}}
	work, err := service.queueWork(ctx)
	if err != nil || len(work) != providerLaneLimit || work[0].ExternalID != "published" {
		t.Fatal("requested candidate lost behind backlog", work, err)
	}
	for _, state := range []string{"published", "protected", "pending"} {
		var due bool
		if err = db.Pool.QueryRow(ctx, `SELECT next_attempt_at<=now() FROM catalog_candidates WHERE external_id=$1`, state).Scan(&due); err != nil {
			t.Fatal(err)
		}
		if due != (state == "published") {
			t.Fatal("changed another candidate", state, due)
		}
	}
}

func TestScheduleRecheckRetriesMappedSourceFailureWithoutUnprotectingIdentity(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	raw, candidate := purpleFixture(t, "go-basics")
	course, _, err := collectPurpleSchool(raw, candidate, netologyNow())
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"purpleschool.ru"}, Courses: []catalog.Course{course}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id) VALUES($1,$2,$3,$4,$5)`, candidate.Adapter, candidate.ExternalID, candidate.URL, course.ID, course.Offers[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state,code,attempted_at,next_attempt_at) VALUES($1,$2,$3,'purple','rejected','source_unavailable',now(),now()+interval '1 day')`, candidate.Adapter, candidate.ExternalID, candidate.URL); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"source_unavailable", "invalid_purpleschool_contract"} {
		if _, err = db.Pool.Exec(ctx, `UPDATE catalog_candidates SET code=$1`, code); err != nil {
			t.Fatal(err)
		}
		if err = ScheduleRecheck(ctx, db, candidate.Adapter, candidate.ExternalID); err != nil {
			t.Fatal("mapped source failure cannot recover", code, err)
		}
	}
	for _, code := range []string{"protected_identity", "identity_mismatch", "pending", "unknown_failure"} {
		if _, err = db.Pool.Exec(ctx, `UPDATE catalog_candidates SET code=$1,next_attempt_at=now()+interval '1 day'`, code); err != nil {
			t.Fatal(err)
		}
		if err = ScheduleRecheck(ctx, db, candidate.Adapter, candidate.ExternalID); err == nil {
			t.Fatal("identity guard or unknown failure bypassed", code)
		}
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE catalog_candidates SET state='protected',code='source_unavailable'`); err != nil {
		t.Fatal(err)
	}
	if err = ScheduleRecheck(ctx, db, candidate.Adapter, candidate.ExternalID); err == nil {
		t.Fatal("protected candidate reopened")
	}
	if _, err = db.Pool.Exec(ctx, `UPDATE catalog_candidates SET state='rejected'; DELETE FROM catalog_identities`); err != nil {
		t.Fatal(err)
	}
	if err = ScheduleRecheck(ctx, db, candidate.Adapter, candidate.ExternalID); err == nil {
		t.Fatal("unmapped rejected candidate reopened")
	}
}
