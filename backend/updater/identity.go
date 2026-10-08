package updater

import (
	"context"
	"errors"

	"devcourse-finder/catalog"
	"github.com/jackc/pgx/v5"
)

var ErrIdentity = errors.New("course identity is protected or ambiguous")

// bindIdentity runs under the catalog publication lock. Discovery reuses
// operator-curated records by canonical URL, not just automatically generated IDs.
// Multiple existing tariffs sharing a page are protected rather than collapsed.
func bindIdentity(ctx context.Context, tx pgx.Tx, candidate Candidate, record catalog.Course, old []catalog.Course) (catalog.Course, error) {
	var courseID, offerID string
	err := tx.QueryRow(ctx, "SELECT course_id,offer_id FROM catalog_identities WHERE adapter=$1 AND external_id=$2", candidate.Adapter, candidate.ExternalID).Scan(&courseID, &offerID)
	if err != nil && err != pgx.ErrNoRows {
		return record, err
	}
	matched := -1
	for i, c := range old {
		source, e := canonical(c.Source)
		if courseID != "" && c.ID == courseID || courseID == "" && e == nil && source == candidate.URL {
			if matched >= 0 {
				return record, ErrIdentity
			}
			matched = i
		}
	}
	if matched >= 0 {
		current := old[matched]
		record.ID = current.ID
		record.Slug = current.Slug
		record.Source = current.Source
		offers := []catalog.Offer{}
		for _, o := range current.Offers {
			clean, e := canonical(o.URL)
			if offerID != "" && o.ID == offerID || offerID == "" && e == nil && clean == candidate.URL {
				offers = append(offers, o)
			}
		}
		if len(offers) != 1 {
			return record, ErrIdentity
		}
		record.Offers[0].ID = offers[0].ID
		record.Offers[0].URL = offers[0].URL
	} else if courseID != "" {
		return record, ErrIdentity
	}
	// Reject an identity already used by another external course, or a fixed ID
	// already used for another URL. All checks precede any course publication.
	for _, current := range old {
		if current.ID == record.ID && matched < 0 {
			return record, ErrIdentity
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id)
 VALUES($1,$2,$3,$4,$5) ON CONFLICT(adapter,external_id) DO NOTHING`, candidate.Adapter, candidate.ExternalID, candidate.URL, record.ID, record.Offers[0].ID)
	return record, err
}
