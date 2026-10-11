package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"github.com/jackc/pgx/v5"
)

// Older PurpleSchool publication marked a mentor chat as personal mentoring.
// Reclassify only values whose last relevant audit writer was the collector.
// A later operator import, including an unchanged explicit confirmation, wins.
func restorePurpleLearningOwnership(ctx context.Context, tx pgx.Tx, courses []catalog.Course) ([]catalog.Course, error) {
	out := append([]catalog.Course(nil), courses...)
	for i := range out {
		if out[i].Provider != "PurpleSchool" {
			continue
		}
		out[i].Offers = append([]catalog.Offer(nil), out[i].Offers...)
		for j := range out[i].Offers {
			offer := &out[i].Offers[j]
			if offer.LearningConditions != nil || offer.LearningOwnership != 0 || offer.SupportKnown == nil || !*offer.SupportKnown {
				continue
			}
			var operator string
			var values []byte
			err := tx.QueryRow(ctx, `SELECT audit.operator,jsonb_build_object('review',a.offer->'review','mentor',a.offer->'mentor','support_known',a.offer->'support_known')
    FROM imports audit
    CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(audit.after_data->'courses')='array' THEN audit.after_data->'courses' ELSE '[]'::jsonb END) course
    CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(course->'offers')='array' THEN course->'offers' ELSE '[]'::jsonb END) a(offer)
    WHERE course->>'id'=$1 AND a.offer->>'id'=$2 AND (
      audit.operator NOT IN ('automatic-catalog-discovery','automatic-catalog-publisher','automatic-catalog-updater') OR
      jsonb_build_array(a.offer->'review',a.offer->'mentor',a.offer->'support_known') IS DISTINCT FROM (
       SELECT jsonb_build_array(b.offer->'review',b.offer->'mentor',b.offer->'support_known')
       FROM jsonb_array_elements(CASE WHEN jsonb_typeof(audit.before_data)='array' THEN audit.before_data ELSE '[]'::jsonb END) before_course
       CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(before_course->'offers')='array' THEN before_course->'offers' ELSE '[]'::jsonb END) b(offer)
       WHERE before_course->>'id'=$1 AND b.offer->>'id'=$2 LIMIT 1))
    ORDER BY audit.id DESC LIMIT 1`, out[i].ID, offer.ID).Scan(&operator, &values)
			if err == pgx.ErrNoRows {
				continue
			}
			if err != nil {
				return nil, err
			}
			var previous struct {
				Review, Mentor bool
				Known          *bool `json:"support_known"`
			}
			if json.Unmarshal(values, &previous) != nil {
				return nil, ErrSource
			}
			if (operator == "automatic-catalog-discovery" || operator == "automatic-catalog-publisher" || operator == "automatic-catalog-updater") && previous.Review == offer.Review && previous.Mentor == offer.Mentor && previous.Known != nil && *previous.Known == *offer.SupportKnown {
				offer.LearningOwnership = learningReview | learningMentor
			}
		}
	}
	return out, nil
}
