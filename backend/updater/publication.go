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
	if candidate.Adapter == "yandex" && len(o.PracticumTariffs) > 0 {
		return mergePracticumGroup(ctx, tx, old, candidate, record, o, observedAt, code)
	}
	if verifiedTariffAdapter(candidate.Adapter) {
		return mergeVerifiedTariffs(ctx, tx, old, candidate, record, o, observedAt, code)
	}
	if candidate.Adapter == "purpleschool" {
		return mergePurpleSchool(ctx, tx, old, candidate, record, o, observedAt, code)
	}
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
	if candidate.Adapter == "skypro" {
		if o.SkyproProductID <= 0 {
			return nil, ErrSource
		}
		var product *int64
		err := tx.QueryRow(ctx, "SELECT skypro_product_id FROM catalog_identities WHERE adapter=$1 AND external_id=$2", candidate.Adapter, candidate.ExternalID).Scan(&product)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if product != nil && *product != o.SkyproProductID {
			*code = "protected_identity"
			_, err = tx.Exec(ctx, "DELETE FROM updater_candidates WHERE source_id IN (SELECT course_id FROM catalog_identities WHERE adapter=$1 AND external_id=$2)", candidate.Adapter, candidate.ExternalID)
			return nil, err
		}
	}
	if candidate.Adapter == "yandex" {
		if !productUUID.MatchString(o.ProductID) || !productUUID.MatchString(o.ProfessionID) {
			return nil, ErrSource
		}
		var product, profession *string
		err := tx.QueryRow(ctx, "SELECT product_id,profession_id FROM catalog_identities WHERE adapter=$1 AND external_id=$2", candidate.Adapter, candidate.ExternalID).Scan(&product, &profession)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if product != nil && *product != o.ProductID || profession != nil && *profession != o.ProfessionID {
			// An identity-rejected read interrupts consecutive confirmation just
			// like a failed fetch. Resolve the persisted (possibly curated) ID.
			if _, err := tx.Exec(ctx, `DELETE FROM updater_candidates WHERE source_id IN
                (SELECT course_id FROM catalog_identities WHERE adapter=$1 AND external_id=$2)`, candidate.Adapter, candidate.ExternalID); err != nil {
				return nil, err
			}
			*code = "protected_identity"
			return nil, nil
		}
	}
	if candidate.Adapter == "netology" {
		if o.NetologyFamilyID <= 0 || o.NetologyProgramID <= 0 {
			return nil, ErrSource
		}
		var family *int64
		err := tx.QueryRow(ctx, "SELECT netology_family_id FROM catalog_identities WHERE adapter=$1 AND external_id=$2", candidate.Adapter, candidate.ExternalID).Scan(&family)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		if family != nil && *family != o.NetologyFamilyID {
			if _, err = tx.Exec(ctx, `DELETE FROM updater_candidates WHERE source_id IN (SELECT course_id FROM catalog_identities WHERE adapter=$1 AND external_id=$2)`, candidate.Adapter, candidate.ExternalID); err != nil {
				return nil, err
			}
			*code = "protected_identity"
			return nil, nil
		}
	}
	resolved, err := bindIdentity(ctx, tx, candidate, record, old)
	if errors.Is(err, ErrIdentity) {
		*code = "protected_identity"
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	mergeObservation := o
	if candidate.Adapter == "rsschool" || candidate.Adapter == "htmlacademy" || candidate.Adapter == "skypro" || candidate.Adapter == "javarush" {
		mergeObservation.ValidUntil = nil
	} // Rolling lease must not prevent closure confirmation.
	courses, err := merge(ctx, tx, old, resolved, resolved.ID, mergeObservation, observedAt, code)
	if (candidate.Adapter == "rsschool" || candidate.Adapter == "htmlacademy" || candidate.Adapter == "skypro" || candidate.Adapter == "javarush") && len(courses) > 0 {
		for i := range courses[0].Offers {
			if courses[0].Offers[i].ID == resolved.Offers[0].ID {
				courses[0].Offers[i].ValidUntil = o.ValidUntil
			}
		}
	}
	if err == nil && len(courses) > 0 && candidate.Adapter == "yandex" {
		_, err = tx.Exec(ctx, "UPDATE catalog_identities SET product_id=$3,profession_id=$4 WHERE adapter=$1 AND external_id=$2", candidate.Adapter, candidate.ExternalID, o.ProductID, o.ProfessionID)
	}
	if err == nil && len(courses) > 0 && candidate.Adapter == "netology" {
		_, err = tx.Exec(ctx, "UPDATE catalog_identities SET netology_family_id=$3,netology_program_id=$4 WHERE adapter=$1 AND external_id=$2", candidate.Adapter, candidate.ExternalID, o.NetologyFamilyID, o.NetologyProgramID)
	}
	if err == nil && len(courses) > 0 && candidate.Adapter == "skypro" {
		_, err = tx.Exec(ctx, "UPDATE catalog_identities SET skypro_product_id=$3 WHERE adapter=$1 AND external_id=$2", candidate.Adapter, candidate.ExternalID, o.SkyproProductID)
	}
	return courses, err
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
