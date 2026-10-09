package updater

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"devcourse-finder/catalog"
	"github.com/jackc/pgx/v5"
)

func validPurpleRecord(e QueuedObservation) bool {
	o := e.Observation
	if o.PurpleCourseID <= 0 || len(o.PurpleTariffs) < 1 || len(o.PurpleTariffs) > 3 || len(e.Record.Offers) != len(o.PurpleTariffs) || o.Enrollment != "continuous" || o.Schedule != "flexible" || o.Price == nil || o.PriceUnknown || o.ValidUntil == nil || !o.ValidUntil.After(e.ObservedAt) || o.ValidUntil.After(e.ObservedAt.Add(26*time.Hour+time.Second)) {
		return false
	}
	ids := map[int64]bool{}
	categories := map[string]bool{}
	for i, t := range o.PurpleTariffs {
		r := e.Record.Offers[i]
		if t.ID <= 0 || ids[t.ID] || categories[t.Category] || t.Name == "" || t.Price < 0 || r.ID != e.Record.ID+"-t"+strconv.FormatInt(t.ID, 10) || r.URL != e.Candidate.URL || r.Name != t.Name || r.Price == nil || *r.Price != t.Price || r.PriceKind != "exact" || r.Free != (t.Price == 0) || r.SupportKnown == nil || !*r.SupportKnown || r.Review != t.Review || r.Mentor != t.Mentor || r.Enrollment != o.Enrollment || r.Schedule != o.Schedule || r.ValidUntil == nil || !r.ValidUntil.Equal(*o.ValidUntil) {
			return false
		}
		switch t.Category {
		case "free":
			if len(o.PurpleTariffs) != 1 || t.Price != 0 || t.Review || t.Mentor {
				return false
			}
		case "basic", "aiMentor":
			if t.Price <= 0 || t.Review || t.Mentor {
				return false
			}
		case "teamProject":
			if t.Price <= 0 || !t.Review || !t.Mentor {
				return false
			}
		default:
			return false
		}
		ids[t.ID] = true
		categories[t.Category] = true
	}
	return *o.Price == o.PurpleTariffs[0].Price
}
func mergePurpleSchool(ctx context.Context, tx pgx.Tx, old []catalog.Course, candidate Candidate, record catalog.Course, o Observation, now time.Time, code *string) ([]catalog.Course, error) {
	binding := struct {
		CourseID int64            `json:"course_id"`
		Tariffs  map[string]int64 `json:"tariffs"`
	}{o.PurpleCourseID, map[string]int64{}}
	for _, t := range o.PurpleTariffs {
		binding.Tariffs[t.Category] = t.ID
	}
	raw, _ := json.Marshal(binding)
	var saved []byte
	var storedID string
	err := tx.QueryRow(ctx, "SELECT course_id,purple_binding FROM catalog_identities WHERE adapter=$1 AND external_id=$2", candidate.Adapter, candidate.ExternalID).Scan(&storedID, &saved)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	protect := func() ([]catalog.Course, error) {
		*code = "protected_identity"
		_, err := tx.Exec(ctx, `DELETE FROM updater_candidates WHERE starts_with(source_id,$1)`, record.ID+":")
		return nil, err
	}
	if saved != nil {
		previous := struct {
			CourseID int64            `json:"course_id"`
			Tariffs  map[string]int64 `json:"tariffs"`
		}{}
		if json.Unmarshal(saved, &previous) != nil || previous.CourseID != binding.CourseID || len(previous.Tariffs) != len(binding.Tariffs) || storedID != record.ID {
			return protect()
		}
		for category, id := range binding.Tariffs {
			if previous.Tariffs[category] != id {
				return protect()
			}
		}
	}
	var existing *catalog.Course
	for i := range old {
		if old[i].ID == record.ID || old[i].Source == candidate.URL {
			if existing != nil {
				return protect()
			}
			existing = &old[i]
		}
	}
	if existing != nil {
		if saved == nil || existing.ID != record.ID || existing.Source != candidate.URL || len(existing.Offers) != len(record.Offers) {
			return protect()
		}
		for _, offer := range record.Offers {
			found := false
			for _, current := range existing.Offers {
				if current.ID == offer.ID && current.URL == offer.URL {
					found = true
				}
			}
			if !found {
				return protect()
			}
		}
	} else if saved != nil {
		return protect()
	}
	working := append([]catalog.Course(nil), old...)
	var result []catalog.Course
	pending := false
	for i, tariff := range o.PurpleTariffs {
		template := record
		template.Offers = append([]catalog.Offer(nil), record.Offers...)
		template.Offers[0], template.Offers[i] = template.Offers[i], template.Offers[0]
		// The rolling verification lease is excluded from the confirmation hash;
		// otherwise a second scheduled read could never confirm the same price.
		observation := Observation{Price: &tariff.Price, Enrollment: o.Enrollment, Schedule: o.Schedule}
		local := "verified"
		courses, err := merge(ctx, tx, working, template, record.ID+":"+strconv.FormatInt(tariff.ID, 10), observation, now, &local)
		if err != nil {
			return nil, err
		}
		if local == "pending_confirmation" {
			pending = true
			continue
		}
		if local != "verified" {
			*code = local
			return nil, nil
		}
		if len(courses) > 0 {
			c := courses[0]
			for j := range c.Offers {
				if c.Offers[j].ID == template.Offers[0].ID {
					c.Offers[j].ValidUntil = o.ValidUntil
					c.Offers[j].Name = tariff.Name
					c.Offers[j].SupportKnown = template.Offers[0].SupportKnown
					c.Offers[j].Review = tariff.Review
					c.Offers[j].Mentor = tariff.Mentor
				}
			}
			result = []catalog.Course{c}
			replaced := false
			for j := range working {
				if working[j].ID == c.ID {
					working[j] = c
					replaced = true
				}
			}
			if !replaced {
				working = append(working, c)
			}
		}
	}
	if pending {
		*code = "pending_confirmation"
	}
	if len(result) > 0 {
		_, err = tx.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id,purple_binding) VALUES($1,$2,$3,$4,$5,$6::jsonb) ON CONFLICT(adapter,external_id) DO NOTHING`, candidate.Adapter, candidate.ExternalID, candidate.URL, record.ID, record.Offers[0].ID, raw)
	}
	return result, err
}
