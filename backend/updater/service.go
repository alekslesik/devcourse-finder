package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"
	_ "time/tzdata"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
)

type Service struct {
	DB     *store.DB
	Config Config
	Client *http.Client
	Worker string
}
type Result struct {
	ID        int64  `json:"run_id"`
	Published int    `json:"published"`
	Queued    int    `json:"queued"`
	Failed    int    `json:"failed"`
	Status    string `json:"status"`
}

var ErrBusy = errors.New("another collection is running")

func (s *Service) Run(ctx context.Context) (result Result, err error) {
	if err = ValidateWorker(s.Worker); err != nil {
		return result, err
	}
	if s.Worker == "publisher" {
		return s.Publish(ctx)
	}
	filtered, filterErr := s.Config.ForWorker(s.Worker)
	if filterErr != nil {
		return result, filterErr
	}
	copyService := *s
	copyService.Config = filtered
	s = &copyService
	if err = s.Config.Validate(); err != nil {
		return result, err
	}
	// The lock lives in a dedicated transaction, so pool reuse, cancellation or
	// process death cannot leak a session lock. HTTP requests use no catalog lock.
	lock, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		lock.Rollback(cleanup)
	}()
	var acquired bool
	guardQuery := "SELECT pg_try_advisory_xact_lock(829105)"
	if s.Worker != "" {
		guardQuery = "SELECT pg_try_advisory_xact_lock_shared(829105)"
	}
	if err = lock.QueryRow(ctx, guardQuery).Scan(&acquired); err != nil {
		return result, err
	}
	if !acquired {
		return result, ErrBusy
	}
	if s.Worker != "" {
		key := int64(829106)
		if s.Worker == "pages" {
			key = 829107
		}
		if err = lock.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock($1)", key).Scan(&acquired); err != nil {
			return result, err
		}
		if !acquired {
			return result, ErrBusy
		}
	}
	if err = s.DB.Pool.QueryRow(ctx, "INSERT INTO updater_runs(worker) VALUES($1) RETURNING id", workerName(s.Worker)).Scan(&result.ID); err != nil {
		return result, err
	}
	result.Status = "failed"
	defer func() {
		finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, finishErr := s.DB.Pool.Exec(finish, "UPDATE updater_runs SET finished_at=now(),status=$2,published=$3,failed=$4,queued=$5 WHERE id=$1", result.ID, result.Status, result.Published, result.Failed, result.Queued)
		if err == nil {
			err = finishErr
		}
	}()
	client := s.Client
	if client == nil {
		client = NewClient()
		defer client.CloseIdleConnections()
	}
	checked := len(s.Config.Sources)
	for _, source := range s.Config.Sources {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		template := s.Config.Template(source.CourseID)
		observedAt := time.Now().UTC().Truncate(time.Microsecond)
		body, digest, fetchErr := fetch(ctx, client, endpoint(source, template.Source))
		code := "source_unavailable"
		var observation Observation
		if fetchErr == nil {
			observation, fetchErr = parse(body, source, template.Source, observedAt)
			code = rejectionCode(fetchErr, "invalid_source")
		}
		verified := fetchErr == nil
		var readEvent *QueuedObservation
		if s.Worker != "" {
			candidate, ok := candidateURL(sourceAdapter(source.Adapter), template.Source, "")
			if !ok {
				return result, ErrSource
			}
			if verified {
				readEvent = &QueuedObservation{Kind: "curated", Candidate: candidate, SourceID: source.CourseID, Record: template, Observation: observation, ObservedAt: observedAt, Digest: digest}
				code = "queued"
			} else {
				readEvent = &QueuedObservation{Kind: "failure", Candidate: candidate, SourceID: source.CourseID, FailureCode: code, Digest: digest, ObservedAt: observedAt}
				result.Failed++
			}
			result.Queued++
		} else if verified {
			code = "verified"
			n, publishErr := s.DB.UpdateVerified(ctx, s.Config.Templates.Domains, "automatic-catalog-updater", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
				return merge(ctx, tx, old, template, source.CourseID, observation, observedAt, &code)
			})
			if publishErr != nil {
				return result, publishErr
			}
			result.Published += n
		} else {
			result.Failed++
			// A failed intervening observation breaks consecutive confirmation.
			if _, err = s.DB.Pool.Exec(ctx, "DELETE FROM updater_candidates WHERE source_id=$1", source.CourseID); err != nil {
				return result, err
			}
		}
		var verifiedAt *time.Time
		if verified {
			verifiedAt = &observedAt
		}
		var failures int
		updateSource := func(ctx context.Context, tx pgx.Tx) error {
			return tx.QueryRow(ctx, `INSERT INTO updater_sources(source_id,attempted_at,verified_at,code,failures,evidence_sha256)
   VALUES($1,$2,$3,$4,$5,NULLIF($6,'')) ON CONFLICT(source_id) DO UPDATE SET
   attempted_at=EXCLUDED.attempted_at,verified_at=COALESCE(EXCLUDED.verified_at,updater_sources.verified_at),
   code=EXCLUDED.code,failures=CASE WHEN EXCLUDED.failures=0 THEN 0 ELSE updater_sources.failures+1 END,
   evidence_sha256=COALESCE(EXCLUDED.evidence_sha256,updater_sources.evidence_sha256) RETURNING failures`, source.CourseID, observedAt, verifiedAt, code, boolInt(!verified), digest).Scan(&failures)
		}
		if readEvent != nil {
			err = s.enqueue(ctx, *readEvent, updateSource)
		} else {
			err = s.DB.Pool.QueryRow(ctx, `INSERT INTO updater_sources(source_id,attempted_at,verified_at,code,failures,evidence_sha256)
   VALUES($1,$2,$3,$4,$5,NULLIF($6,'')) ON CONFLICT(source_id) DO UPDATE SET
   attempted_at=EXCLUDED.attempted_at,verified_at=COALESCE(EXCLUDED.verified_at,updater_sources.verified_at),
   code=EXCLUDED.code,failures=CASE WHEN EXCLUDED.failures=0 THEN 0 ELSE updater_sources.failures+1 END,
   evidence_sha256=COALESCE(EXCLUDED.evidence_sha256,updater_sources.evidence_sha256) RETURNING failures`, source.CourseID, observedAt, verifiedAt, code, boolInt(!verified), digest).Scan(&failures)
		}
		if err != nil {
			return result, err
		}
		level := slog.LevelInfo
		if !verified {
			level = slog.LevelWarn
		}
		if failures >= 3 {
			level = slog.LevelError
		}
		slog.Log(ctx, level, "catalog source checked", "source_id", source.CourseID, "code", code, "consecutive_failures", failures, "collection_id", result.ID)
	}
	if len(s.Config.Discovery) > 0 {
		feedStats, discoveryErr := s.discover(ctx, client)
		result.Failed += feedStats.Failed
		checked += feedStats.Attempted
		if discoveryErr != nil {
			return result, discoveryErr
		}
		batchStats, batchErr := s.processBatch(ctx, client, s.publishDiscovered)
		result.Queued += batchStats.Queued
		result.Published += batchStats.Published
		result.Failed += batchStats.Failed
		checked += batchStats.Attempted
		if batchErr != nil {
			return result, batchErr
		}
	}
	result.Status = "completed"
	if result.Failed > 0 {
		result.Status = "partial"
	}
	if result.Failed >= checked && result.Published == 0 {
		result.Status = "failed"
	}
	slog.Info("catalog collection finished", "collection_id", result.ID, "status", result.Status, "published", result.Published, "queued", result.Queued, "worker", workerName(s.Worker), "failed", result.Failed)
	return result, nil
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func merge(ctx context.Context, tx pgx.Tx, old []catalog.Course, template catalog.Course, sourceID string, o Observation, now time.Time, code *string) ([]catalog.Course, error) {
	c := template
	found := false
	for _, current := range old {
		if current.ID == template.ID {
			c = current
			found = true
			break
		}
	}
	if found && (c.Status != "published" || c.Demo || c.Source != template.Source) {
		*code = "protected_course"
		return nil, nil
	}
	for _, other := range old {
		for _, offer := range other.Offers {
			if other.ID != template.ID && offer.ID == template.Offers[0].ID {
				*code = "protected_offer"
				return nil, nil
			}
		}
	}
	offerIndex := -1
	for i, current := range c.Offers {
		if current.ID == template.Offers[0].ID {
			offerIndex = i
		}
	}
	if offerIndex < 0 {
		*code = "protected_offer"
		return nil, nil
	}
	c.Offers = append([]catalog.Offer(nil), c.Offers...)
	current := c.Offers[offerIndex]
	if found && current.URL != template.Offers[0].URL {
		*code = "protected_offer"
		return nil, nil
	}
	// A first publication needs live availability and price evidence; templates
	// alone are never imported, and unknown enrollment cannot make them public.
	if !found && (o.Enrollment != "open" && o.Enrollment != "continuous" || o.Price == nil && !o.PriceUnknown) {
		*code = "insufficient_initial_evidence"
		return nil, nil
	}
	if needsConfirmation(current, o) {
		accepted, err := confirm(ctx, tx, sourceID, o, now)
		if err != nil {
			return nil, err
		}
		if !accepted {
			*code = "pending_confirmation"
			return nil, nil
		}
	} else if _, err := tx.Exec(ctx, "DELETE FROM updater_candidates WHERE source_id=$1", sourceID); err != nil {
		return nil, err
	}
	if o.Price != nil {
		current.Price = o.Price
		current.PriceKind = "exact"
		current.Free = *o.Price == 0
		current.PriceCheckedAt = now
		current.ValidUntil = o.ValidUntil
	}
	if o.PriceUnknown {
		current.Price = nil
		current.PriceKind = "unknown"
		current.Free = false
		// No price was verified: never advance price_checked_at.
	}
	if o.Enrollment != "" {
		current.Enrollment = o.Enrollment
	}
	if o.Schedule != "" {
		current.Schedule = o.Schedule
	}
	c.CheckedAt = now
	c.Offers[offerIndex] = current
	return []catalog.Course{c}, nil
}
func needsConfirmation(old catalog.Offer, o Observation) bool {
	if o.Enrollment == "closed" && old.Enrollment != "closed" {
		return true
	}
	if o.Price == nil || old.Price == nil || old.PriceKind != "exact" || *old.Price == *o.Price {
		return false
	}
	// Free/paid transitions and changes greater than 50% are checked again in
	// another scheduled window. Small exact-price corrections apply immediately.
	a, b := *old.Price, *o.Price
	return a == 0 || b == 0 || b*2 < a || b*2 > a*3
}
func confirm(ctx context.Context, tx pgx.Tx, id string, o Observation, now time.Time) (bool, error) {
	raw, _ := json.Marshal(o)
	sum := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(sum[:])
	var previous string
	var observed time.Time
	var count int
	err := tx.QueryRow(ctx, "SELECT fingerprint,observed_at,confirmations FROM updater_candidates WHERE source_id=$1", id).Scan(&previous, &observed, &count)
	if err != nil && err != pgx.ErrNoRows {
		return false, err
	}
	if err == nil && previous == fingerprint && now.Sub(observed) >= 6*time.Hour && now.Sub(observed) <= 26*time.Hour {
		_, err = tx.Exec(ctx, "DELETE FROM updater_candidates WHERE source_id=$1", id)
		return true, err
	}
	if err == nil && previous == fingerprint && now.Sub(observed) < 6*time.Hour {
		return false, nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO updater_candidates VALUES($1,$2,$3,1) ON CONFLICT(source_id) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,observed_at=EXCLUDED.observed_at,confirmations=1`, id, fingerprint, now)
	return false, err
}

// NextRun uses Moscow wall-clock slots; no OS tzdata is required by the image.
func NextRun(now time.Time) time.Time {
	location, _ := time.LoadLocation("Europe/Moscow")
	local := now.In(location)
	for _, hour := range []int{9, 21} {
		next := time.Date(local.Year(), local.Month(), local.Day(), hour, 0, 0, 0, location)
		if next.After(now) {
			return next
		}
	}
	return time.Date(local.Year(), local.Month(), local.Day()+1, 9, 0, 0, 0, location)
}
func heartbeat(ctx context.Context, db *store.DB) error { return heartbeatWorker(ctx, db, "") }
func Health(ctx context.Context, db *store.DB) error    { return HealthWorker(ctx, db, "") }
func (s *Service) Serve(ctx context.Context) error {
	if err := ValidateWorker(s.Worker); err != nil {
		return err
	}
	if err := s.Config.Validate(); err != nil {
		return err
	}
	if err := heartbeatWorker(ctx, s.DB, s.Worker); err != nil {
		return err
	}
	beats, stopBeats := context.WithCancel(ctx)
	defer stopBeats()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-beats.Done():
				return
			case <-ticker.C:
				pulse, cancel := context.WithTimeout(beats, 5*time.Second)
				if heartbeatWorker(pulse, s.DB, s.Worker) != nil && beats.Err() == nil {
					slog.Error("updater heartbeat failed", "code", "heartbeat_failed")
				}
				cancel()
			}
		}
	}()
	if s.Worker == "publisher" {
		for {
			run, cancel := context.WithTimeout(ctx, 10*time.Minute)
			_, err := s.Publish(run)
			cancel()
			if err != nil && !errors.Is(err, ErrBusy) && ctx.Err() == nil {
				slog.Error("catalog publication failed", "code", "publication_failed")
			}
			timer := time.NewTimer(10 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
	}
	for {
		// First run fills an empty verified catalog automatically. Restarts are
		// deduplicated using the latest started run and the preceding schedule slot.
		now := time.Now()
		due := NextRun(now.Add(-12 * time.Hour))
		var last *time.Time
		if err := s.DB.Pool.QueryRow(ctx, "SELECT max(started_at) FROM updater_runs WHERE worker=$1", workerName(s.Worker)).Scan(&last); err != nil {
			return err
		}
		if last == nil || last.Before(due) {
			run, cancel := context.WithTimeout(ctx, 10*time.Minute)
			_, err := s.Run(run)
			cancel()
			if err != nil && !errors.Is(err, ErrBusy) && ctx.Err() == nil {
				slog.Error("catalog collection failed", "code", "collection_failed")
			}
		}
		next := NextRun(time.Now())
		slog.Info("next catalog collection scheduled", "at", next.Format(time.RFC3339), "timezone", "Europe/Moscow")
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
func Status(ctx context.Context, db *store.DB, w io.Writer) error {
	runs, err := db.Pool.Query(ctx, `SELECT id,worker,started_at,finished_at,status,published,failed,queued FROM (SELECT DISTINCT ON(worker) * FROM updater_runs ORDER BY worker,id DESC) latest ORDER BY worker`)
	if err != nil {
		return err
	}
	for runs.Next() {
		var id int64
		var worker, status string
		var started time.Time
		var finished *time.Time
		var published, failed, queued int
		if err = runs.Scan(&id, &worker, &started, &finished, &status, &published, &failed, &queued); err != nil {
			runs.Close()
			return err
		}
		if err = json.NewEncoder(w).Encode(map[string]any{"run_id": id, "worker": worker, "started_at": started, "finished_at": finished, "status": status, "published": published, "failed": failed, "queued": queued}); err != nil {
			runs.Close()
			return err
		}
	}
	if err = runs.Err(); err != nil {
		runs.Close()
		return err
	}
	runs.Close()
	if err = observationStatus(ctx, db, w); err != nil {
		return err
	}
	rows, err := db.Pool.Query(ctx, "SELECT source_id,attempted_at,verified_at,code,failures FROM updater_sources ORDER BY source_id")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, code string
		var attempted time.Time
		var verified *time.Time
		var failures int
		if err = rows.Scan(&id, &attempted, &verified, &code, &failures); err != nil {
			return err
		}
		if err = json.NewEncoder(w).Encode(map[string]any{"source_id": id, "attempted_at": attempted, "verified_at": verified, "code": code, "consecutive_failures": failures}); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	return coverage(ctx, db, w)
}
