package updater

import (
	"context"
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
