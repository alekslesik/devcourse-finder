package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"github.com/jackc/pgx/v5"
)

const observationQueueLimit = 50000
const publisherBatchLimit = 500

func failureCodeAllowed(code string) bool {
	switch code {
	case "invalid_javarush_contract", "invalid_skypro_contract", "invalid_skillfactory_contract", "invalid_skillbox_contract", "invalid_htmlacademy_page", "invalid_htmlacademy_payment", "unsupported_htmlacademy_format", "invalid_rsschool_contract", "unverified_rsschool_free", "invalid_purpleschool_contract", "invalid_netology_page", "invalid_netology_payload", "invalid_netology_price", "unverified_netology_payment", "invalid_practicum_page", "invalid_practicum_payload", "invalid_practicum_price", "source_unavailable", "invalid_course", "invalid_source", "identity_mismatch", "invalid_payload", "unsupported_content_language", "insufficient_curriculum", "missing_visibility_or_price_flags", "private_or_censored_course", "inactive_course", "invalid_price_evidence", "unverified_enrollment", "invalid_course_text", "unsupported_or_ambiguous_language", "invalid_structured_payload", "missing_course_schema", "unverified_course_offer", "ambiguous_course_schema":
		return true
	default:
		return false
	}
}

var evidenceDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

// QueuedObservation contains normalized public facts, never raw HTML, cookies,
// credentials or arbitrary underlying error messages. The publisher revalidates it.
type QueuedObservation struct {
	Version     int            `json:"version"`
	Kind        string         `json:"kind"`
	Candidate   Candidate      `json:"candidate"`
	SourceID    string         `json:"source_id,omitempty"`
	Record      catalog.Course `json:"record"`
	Observation Observation    `json:"observation"`
	ObservedAt  time.Time      `json:"observed_at"`
	Digest      string         `json:"digest,omitempty"`
	FailureCode string         `json:"failure_code,omitempty"`
}

func (e QueuedObservation) validate(config Config) error {
	identity, ok := candidateURL(e.Candidate.Adapter, e.Candidate.URL, e.Candidate.FeedID)
	if e.Version != 1 || !ok || identity.URL != e.Candidate.URL || identity.ExternalID != e.Candidate.ExternalID || e.ObservedAt.IsZero() {
		return ErrSource
	}
	if e.Kind != "curated" && e.Kind != "discovered" && e.Kind != "failure" {
		return ErrSource
	}
	if e.SourceID != "" {
		valid := false
		for _, source := range config.Sources {
			if source.CourseID == e.SourceID && sourceAdapter(source.Adapter) == e.Candidate.Adapter && config.Template(source.CourseID).Source == e.Candidate.URL {
				valid = true
			}
		}
		if !valid {
			return ErrSource
		}
	} else {
		valid := false
		for _, feed := range config.Discovery {
			if feed.ID == e.Candidate.FeedID && feed.Adapter == e.Candidate.Adapter {
				valid = true
			}
		}
		if !valid {
			return ErrSource
		}
	}
	if e.Digest != "" && !evidenceDigest.MatchString(e.Digest) {
		return ErrSource
	}
	if e.Kind == "failure" {
		if !failureCodeAllowed(e.FailureCode) {
			return ErrSource
		}
		return nil
	}
	if e.Kind == "curated" && e.SourceID == "" || e.Kind == "discovered" && e.SourceID != "" || !evidenceDigest.MatchString(e.Digest) {
		return ErrSource
	}
	if e.Record.Source != e.Candidate.URL || e.Record.Status != "published" || e.Record.Demo || (e.Candidate.Adapter != "purpleschool" && !(e.Candidate.Adapter == "yandex" && len(e.Observation.PracticumTariffs) > 0) && !verifiedTariffAdapter(e.Candidate.Adapter) && len(e.Record.Offers) != 1) || len(e.Record.Offers) == 0 || e.Record.Offers[0].URL != e.Candidate.URL {
		return ErrSource
	}
	if e.Kind == "curated" {
		template := config.Template(e.SourceID)
		if e.Record.ID != template.ID || e.Record.Offers[0].ID != template.Offers[0].ID {
			return ErrSource
		}
	} else {
		expectedID := e.Candidate.Adapter + "-" + e.Candidate.ExternalID
		if len(expectedID) > 70 {
			expectedID = e.Candidate.Adapter + "-" + fingerprint([]byte(e.Candidate.ExternalID))[:24]
		}
		if e.Record.ID != expectedID || (e.Candidate.Adapter != "purpleschool" && !(e.Candidate.Adapter == "yandex" && len(e.Observation.PracticumTariffs) > 0) && !verifiedTariffAdapter(e.Candidate.Adapter) && e.Record.Offers[0].ID != expectedID+"-course") {
			return ErrSource
		}
	}
	raw, err := json.Marshal(catalog.Dataset{Domains: []string{providerHosts[e.Candidate.Adapter]}, Courses: []catalog.Course{e.Record}})
	if err != nil {
		return err
	}
	if _, err = catalog.Decode(bytes.NewReader(raw)); err != nil {
		return ErrSource
	}
	o := e.Observation
	if e.Candidate.Adapter == "javarush" {
		offer := e.Record.Offers[0]
		if !o.Billing.Valid() || o.Billing.Currency != "USD" || o.Billing.Interval != "month" || offer.Billing == nil || *o.Billing != *offer.Billing || !o.PriceUnknown || o.Price != nil || o.Enrollment != "continuous" || o.Schedule != "flexible" || o.ValidUntil == nil || !o.ValidUntil.After(e.ObservedAt) || o.ValidUntil.After(e.ObservedAt.Add(26*time.Hour+time.Second)) || offer.Price != nil || offer.Free || offer.PriceKind != "unknown" || offer.Enrollment != o.Enrollment || offer.Schedule != o.Schedule || offer.ValidUntil == nil || !offer.ValidUntil.Equal(*o.ValidUntil) {
			return ErrSource
		}
	} else {
		if o.Billing != nil {
			return ErrSource
		}
		for _, offer := range e.Record.Offers {
			if offer.Billing != nil {
				return ErrSource
			}
		}
	}

	if e.Candidate.Adapter == "skypro" {
		offer := e.Record.Offers[0]
		if o.SkyproProductID <= 0 || !o.PriceUnknown || o.Price != nil || (o.Enrollment != "closed" && o.Enrollment != "open") || o.Schedule != "unknown" || o.ValidUntil == nil || !o.ValidUntil.After(e.ObservedAt) || o.ValidUntil.After(e.ObservedAt.Add(26*time.Hour+time.Second)) || offer.Price != nil || offer.Free || offer.PriceKind != "unknown" || offer.Enrollment != o.Enrollment || offer.ValidUntil == nil || !offer.ValidUntil.Equal(*o.ValidUntil) {
			return ErrSource
		}
	} else if o.SkyproProductID != 0 {
		return ErrSource
	}
	if verifiedTariffAdapter(e.Candidate.Adapter) {
		if !validVerifiedTariffs(e) {
			return ErrSource
		}
	} else if o.VerifiedProductID != 0 || len(o.VerifiedTariffs) != 0 {
		return ErrSource
	}
	if e.Candidate.Adapter == "htmlacademy" {
		offer := e.Record.Offers[0]
		if !strings.HasPrefix(e.Candidate.ExternalID, "intensive-") || !o.PriceUnknown || o.Price != nil || o.Enrollment != "continuous" || o.Schedule != "flexible" || o.ValidUntil == nil || !o.ValidUntil.After(e.ObservedAt) || o.ValidUntil.After(e.ObservedAt.Add(26*time.Hour+time.Second)) || offer.Price != nil || offer.Free || offer.PriceKind != "unknown" || offer.Name != "Индивидуальный формат — помесячная оплата" || offer.Enrollment != o.Enrollment || offer.Schedule != o.Schedule || offer.ValidUntil == nil || !offer.ValidUntil.Equal(*o.ValidUntil) || offer.Mentor || offer.Review || offer.SupportKnown == nil || *offer.SupportKnown {
			return ErrSource
		}
	}
	if e.Candidate.Adapter == "rsschool" {
		offer := e.Record.Offers[0]
		if o.Price == nil || *o.Price != 0 || o.PriceUnknown || o.Schedule != "scheduled" || (o.Enrollment != "open" && o.Enrollment != "closed") || o.ValidUntil == nil || !o.ValidUntil.After(e.ObservedAt) || o.ValidUntil.After(e.ObservedAt.Add(26*time.Hour+time.Second)) || !offer.Free || offer.Price == nil || *offer.Price != 0 || offer.Mentor || offer.Review || offer.SupportKnown == nil || *offer.SupportKnown {
			return ErrSource
		}
	}
	if e.Candidate.Adapter == "purpleschool" {
		if !validPurpleRecord(e) {
			return ErrSource
		}
	} else if o.PurpleCourseID != 0 || len(o.PurpleTariffs) > 0 {
		return ErrSource
	}
	if e.Candidate.Adapter == "netology" {
		if o.NetologyFamilyID <= 0 || o.NetologyProgramID <= 0 {
			return ErrSource
		}
	} else if o.NetologyFamilyID != 0 || o.NetologyProgramID != 0 {
		return ErrSource
	}
	if len(o.PracticumTariffs) > 0 {
		if !validPracticumGroup(e) {
			return ErrSource
		}
	}
	if e.Candidate.Adapter != "yandex" && len(o.PracticumTariffs) > 0 {
		return ErrSource
	}
	if e.Candidate.Adapter == "yandex" {
		if !productUUID.MatchString(o.ProductID) || !productUUID.MatchString(o.ProfessionID) {
			return ErrSource
		}
	} else if o.ProductID != "" || o.ProfessionID != "" {
		return ErrSource
	}
	if o.Price != nil && *o.Price < 0 || o.Price != nil && o.PriceUnknown {
		return ErrSource
	}
	for _, value := range []struct {
		value   string
		allowed []string
	}{{o.Enrollment, []string{"", "open", "closed", "continuous"}}, {o.Schedule, []string{"", "flexible", "scheduled", "unknown"}}} {
		valid := false
		for _, allowed := range value.allowed {
			if value.value == allowed {
				valid = true
			}
		}
		if !valid {
			return ErrSource
		}
	}
	return nil
}

func (s *Service) enqueue(ctx context.Context, e QueuedObservation, updates ...func(context.Context, pgx.Tx) error) error {
	e.Version = 1
	e.ObservedAt = e.ObservedAt.UTC().Truncate(time.Microsecond)
	if err := e.validate(s.Config); err != nil {
		return err
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return err
	}
	if len(raw) > 65536 {
		return ErrSource
	}
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(829110)"); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM catalog_observations WHERE processed_at IS NULL").Scan(&count); err != nil {
		return err
	}
	if count >= observationQueueLimit {
		return errors.New("observation queue is full; publication must catch up")
	}
	for _, update := range updates {
		if err = update(ctx, tx); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog_observations(canonical_url,observed_at,kind,payload) VALUES($1,$2,$3,$4) ON CONFLICT(canonical_url,observed_at,kind) DO NOTHING`, e.Candidate.URL, e.ObservedAt, e.Kind, raw); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Publish consumes FIFO observations under one publisher lock. Each catalog
// merge, watermark, candidate status and acknowledgement commit atomically.
func (s *Service) Publish(ctx context.Context) (result Result, err error) {
	result.Status = "completed"
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
	if err = lock.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock_shared(829105)").Scan(&acquired); err != nil {
		return result, err
	}
	if !acquired {
		return result, ErrBusy
	}
	if err = lock.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(829108)").Scan(&acquired); err != nil {
		return result, err
	}
	if !acquired {
		return result, ErrBusy
	}
	for i := 0; i < publisherBatchLimit; i++ {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 5*time.Second {
			break
		}
		var id int64
		var raw []byte
		var url, kind string
		var observedAt time.Time
		err = s.DB.Pool.QueryRow(ctx, `SELECT id,payload,canonical_url,observed_at,kind FROM catalog_observations WHERE processed_at IS NULL ORDER BY id LIMIT 1`).Scan(&id, &raw, &url, &observedAt, &kind)
		if errors.Is(err, pgx.ErrNoRows) {
			err = nil
			break
		}
		if err != nil {
			return result, err
		}
		var e QueuedObservation
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		valid := decoder.Decode(&e) == nil && e.validate(s.Config) == nil && e.Candidate.URL == url && e.Kind == kind && e.ObservedAt.Equal(observedAt)
		code := "invalid_observation"
		n, publishErr := s.DB.UpdateVerified(ctx, []string{providerHosts[e.Candidate.Adapter]}, "automatic-catalog-publisher", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
			var courses []catalog.Course
			if valid {
				code = "verified"
				var last time.Time
				watermarkErr := tx.QueryRow(ctx, "SELECT observed_at FROM catalog_observation_watermarks WHERE canonical_url=$1", url).Scan(&last)
				if watermarkErr != nil && !errors.Is(watermarkErr, pgx.ErrNoRows) {
					return nil, watermarkErr
				}
				stale := e.Observation.ValidUntil != nil && !time.Now().Before(*e.Observation.ValidUntil) || !last.IsZero() && !observedAt.After(last) || time.Since(observedAt) > 48*time.Hour || observedAt.After(time.Now().Add(5*time.Minute))
				// An operator or legacy updater may have recorded a newer verified read.
				for _, c := range old {
					if c.Source == url {
						for _, o := range c.Offers {
							if o.PriceCheckedAt.After(observedAt) {
								stale = true
							}
						}
					}
				}
				if stale {
					code = "stale_observation"
				} else {
					var applyErr error
					switch e.Kind {
					case "failure":
						code = e.FailureCode
						_, applyErr = tx.Exec(ctx, `DELETE FROM updater_candidates WHERE source_id=$1 OR source_id IN (SELECT course_id FROM catalog_identities WHERE canonical_url=$2) OR EXISTS (SELECT 1 FROM catalog_identities i WHERE i.canonical_url=$2 AND i.adapter IN ('purpleschool','skillbox','skillfactory','skypro','yandex') AND starts_with(updater_candidates.source_id,i.course_id||':'))`, e.SourceID, url)
					case "curated":
						courses, applyErr = merge(ctx, tx, old, e.Record, e.SourceID, e.Observation, observedAt, &code)
					case "discovered":
						courses, applyErr = mergeDiscovered(ctx, tx, old, e.Candidate, e.Record, e.Observation, observedAt, &code)
					}
					if applyErr != nil {
						return nil, applyErr
					}
					if _, applyErr = tx.Exec(ctx, `INSERT INTO catalog_observation_watermarks VALUES($1,$2,$3) ON CONFLICT(canonical_url) DO UPDATE SET observed_at=EXCLUDED.observed_at,event_id=EXCLUDED.event_id`, url, observedAt, id); applyErr != nil {
						return nil, applyErr
					}
				}
				if e.Kind != "failure" && code != "stale_observation" {
					state := "rejected"
					delay := 72 * time.Hour
					switch code {
					case "verified", "partial_tariffs":
						state = "published"
						delay = 12 * time.Hour
					case "pending_confirmation":
						state = ""
						delay = 12 * time.Hour
					case "protected_course", "protected_offer", "protected_identity":
						state = "protected"
						delay = 7 * 24 * time.Hour
					}
					if _, updateErr := tx.Exec(ctx, `UPDATE catalog_candidates SET state=CASE WHEN $3='' THEN state ELSE $3 END,code=$4,next_attempt_at=$5 WHERE adapter=$1 AND external_id=$2 AND attempted_at<=$6`, e.Candidate.Adapter, e.Candidate.ExternalID, state, code, time.Now().Add(delay), observedAt); updateErr != nil {
						return nil, updateErr
					}
					if e.SourceID != "" {
						if _, updateErr := tx.Exec(ctx, `UPDATE updater_sources SET code=$2 WHERE source_id=$1 AND attempted_at<=$3`, e.SourceID, code, observedAt); updateErr != nil {
							return nil, updateErr
						}
					}
				}
			}
			if _, ackErr := tx.Exec(ctx, `UPDATE catalog_observations SET processed_at=now(),code=$2 WHERE id=$1 AND processed_at IS NULL`, id, code); ackErr != nil {
				return nil, ackErr
			}
			return courses, nil
		})
		if publishErr != nil {
			return result, publishErr
		}
		result.Published += n
		if code == "invalid_observation" {
			result.Failed++
			result.Status = "partial"
		}
		slog.Info("catalog observation processed", "event_id", id, "code", code, "published", n)
	}
	// Retain recent diagnostics while incrementally trimming old/overflow done rows.
	_, err = s.DB.Pool.Exec(ctx, `DELETE FROM catalog_observations WHERE id IN (SELECT id FROM catalog_observations WHERE processed_at IS NOT NULL AND (processed_at<now()-interval '7 days' OR id<(SELECT id FROM catalog_observations WHERE processed_at IS NOT NULL ORDER BY id DESC OFFSET 25000 LIMIT 1)) ORDER BY id LIMIT 1000)`)
	return result, err
}
