package updater

import (
	"context"
	"devcourse-finder/store"
	"errors"
	"regexp"
	"time"
)

var recheckID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,120}$`)

// Schedule one already published candidate. This changes no catalog facts and
// never creates a URL, unprotects an identity or bypasses normal verification.
func ScheduleRecheck(ctx context.Context, db *store.DB, adapter, id string) error {
	if providerHosts[adapter] == "" || !recheckID.MatchString(id) {
		return errors.New("invalid recheck identity")
	}
	tx, err := db.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tx.Rollback(cleanup)
	}()
	var locked bool
	if err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(829105)").Scan(&locked); err != nil {
		return err
	}
	if !locked {
		return ErrBusy
	}
	// queueWork puts null attempts first within the existing published lane.
	// Merely making the candidate due could leave it behind the lane cap.
	result, err := tx.Exec(ctx, `UPDATE catalog_candidates SET attempted_at=NULL,next_attempt_at=now() WHERE adapter=$1 AND external_id=$2 AND state='published'`, adapter, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("recheck requires one existing published candidate")
	}
	return tx.Commit(ctx)
}
