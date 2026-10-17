package updater

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

func aiPracticumBindingDigest(courseID, offerID string, tariffs []byte) string {
	var bindings map[string]practicumTariffBinding
	if len(tariffs) > 0 && json.Unmarshal(tariffs, &bindings) != nil {
		return ""
	}
	raw, _ := json.Marshal(struct {
		CourseID, OfferID string
		Tariffs           map[string]practicumTariffBinding
	}{courseID, offerID, bindings})
	return fingerprint(raw)
}

func aiPracticumEventIdentityValid(ctx context.Context, tx pgx.Tx, e AIEvent) (bool, error) {
	var offerID string
	var tariffs []byte
	err := tx.QueryRow(ctx, `SELECT offer_id,practicum_tariffs FROM catalog_identities WHERE adapter='yandex' AND external_id=$1 AND canonical_url=$2 AND course_id=$3`, e.Candidate.ExternalID, e.Candidate.URL, e.CourseID).Scan(&offerID, &tariffs)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	digest := aiPracticumBindingDigest(e.CourseID, offerID, tariffs)
	for _, d := range e.Documents {
		if d.BindingDigest != digest {
			return false, nil
		}
	}
	return true, nil
}
