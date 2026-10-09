package updater

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
)

func (s *Service) publishDiscovered(ctx context.Context, candidate Candidate, record catalog.Course, o Observation, digest string) (string, int, error) {
	code := "verified"
	published, err := s.DB.UpdateVerified(ctx, []string{providerHosts[candidate.Adapter]}, "automatic-catalog-discovery", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
		return mergeDiscovered(ctx, tx, old, candidate, record, o, time.Now().UTC(), &code)
	})
	return code, published, err
}
func mergeDiscovered(ctx context.Context, tx pgx.Tx, old []catalog.Course, candidate Candidate, record catalog.Course, o Observation, observedAt time.Time, code *string) ([]catalog.Course, error) {
	existing := false
	for _, c := range old {
		u, e := canonical(c.Source)
		if e == nil && u == candidate.URL {
			existing = true
			break
		}
	}
	if !existing && (o.Enrollment != "open" && o.Enrollment != "continuous" || o.Price == nil && !o.PriceUnknown) {
		*code = "insufficient_initial_evidence"
		return nil, nil
	}
	resolved, err := bindIdentity(ctx, tx, candidate, record, old)
	if errors.Is(err, ErrIdentity) {
		*code = "protected_identity"
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return merge(ctx, tx, old, resolved, resolved.ID, o, observedAt, code)
}

func coverage(ctx context.Context, db *store.DB, w io.Writer) error {
	rows, err := db.Pool.Query(ctx, `SELECT language,provider,count(DISTINCT c.id),count(o.id),
 count(o.id) FILTER(WHERE free AND price=0 AND price_kind='exact' AND price_checked_at>now()-interval '30 days' AND (valid_until IS NULL OR valid_until>now())),
 count(o.id) FILTER(WHERE NOT free),
 count(o.id) FILTER(WHERE price IS NULL OR price_kind<>'exact' OR price_checked_at<=now()-interval '30 days' OR valid_until<=now()),
 min(price_checked_at) FILTER(WHERE price IS NOT NULL)
 FROM courses c JOIN offers o ON o.course_id=c.id WHERE c.status='published' AND NOT c.demo GROUP BY language,provider ORDER BY language,provider`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var language, provider string
		var courses, offers, free, paid, unknown int
		var oldest *time.Time
		if err = rows.Scan(&language, &provider, &courses, &offers, &free, &paid, &unknown, &oldest); err != nil {
			return err
		}
		if err = json.NewEncoder(w).Encode(map[string]any{"coverage": true, "language": language, "provider": provider, "courses": courses, "offers": offers, "confirmed_free_offers": free, "paid_offers": paid, "unknown_or_stale_prices": unknown, "oldest_price_check": oldest}); err != nil {
			return err
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	rows.Close()
	var pending, published, rejected, protected int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='pending'),count(*) FILTER(WHERE state='published'),count(*) FILTER(WHERE state='rejected'),count(*) FILTER(WHERE state='protected') FROM catalog_candidates`).Scan(&pending, &published, &rejected, &protected); err != nil {
		return err
	}
	if err = json.NewEncoder(w).Encode(map[string]any{"queue": true, "pending": pending, "published": published, "rejected": rejected, "protected": protected}); err != nil {
		return err
	}
	feeds, err := db.Pool.Query(ctx, "SELECT feed_id,code,failures,candidates,attempted_at,verified_at FROM discovery_feeds ORDER BY feed_id")
	if err != nil {
		return err
	}
	defer feeds.Close()
	for feeds.Next() {
		var id, code string
		var failures, seen int
		var attempted, verified *time.Time
		if err = feeds.Scan(&id, &code, &failures, &seen, &attempted, &verified); err != nil {
			return err
		}
		if err = json.NewEncoder(w).Encode(map[string]any{"feed_id": id, "code": code, "consecutive_failures": failures, "candidates_seen": seen, "attempted_at": attempted, "verified_at": verified}); err != nil {
			return err
		}
	}
	return feeds.Err()
}
