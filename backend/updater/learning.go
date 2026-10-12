package updater

import (
	"encoding/json"
	"time"

	"devcourse-finder/catalog"
)

const (
	learningDuration = 1 << iota
	learningWorkload
	learningReview
	learningMentor
	learningCommunity
)

// Ownership is private database metadata. Imported curated facts have no owned
// bits. A successful observation replaces/clears only the collector's facts.
func applyLearningConditions(o *catalog.Offer, incoming *catalog.LearningConditions) {
	old := catalog.LearningConditions{}
	if o.LearningConditions != nil {
		old = *o.LearningConditions
	}
	next := catalog.LearningConditions{}
	if incoming != nil {
		next = *incoming
	}
	previous := o.LearningOwnership
	rangeField := func(current **catalog.LearningRange, value *catalog.LearningRange, bit int, curatedLegacy bool) {
		if previous&(bit<<aiOwnershipShift) != 0 {
			return
		}
		if previous&bit == 0 && (*current != nil || curatedLegacy) {
			return
		}
		*current = value
		o.LearningOwnership &^= bit
		if value != nil {
			o.LearningOwnership |= bit
		}
	}
	supportField := func(current **catalog.LearningSupport, value *catalog.LearningSupport, bit int, curatedLegacy bool) {
		if previous&(bit<<aiOwnershipShift) != 0 {
			return
		}
		if previous&bit == 0 && (*current != nil || curatedLegacy) {
			return
		}
		*current = value
		o.LearningOwnership &^= bit
		if value != nil {
			o.LearningOwnership |= bit
		}
	}
	rangeField(&old.Duration, next.Duration, learningDuration, !o.DurationCollected && (o.Weeks != nil || o.DurationMonths != nil))
	rangeField(&old.WeeklyWorkload, next.WeeklyWorkload, learningWorkload, o.Hours != nil)
	legacySupport := o.SupportKnown == nil || *o.SupportKnown
	supportField(&old.Review, next.Review, learningReview, legacySupport)
	supportField(&old.Mentor, next.Mentor, learningMentor, legacySupport)
	supportField(&old.Community, next.Community, learningCommunity, false)
	if (previous|o.LearningOwnership)&learningDuration != 0 {
		o.DurationMonths, o.Weeks = nil, nil
		o.DurationCollected = old.Duration != nil
		if d := old.Duration; d != nil && d.Min == d.Max && d.Max == float64(int(d.Max)) {
			n := int(d.Max)
			if d.Unit == "month" {
				o.DurationMonths = &n
			}
			if d.Unit == "week" {
				o.Weeks = &n
			}
		}
	}
	if (previous|o.LearningOwnership)&learningWorkload != 0 {
		o.Hours = nil
		if w := old.WeeklyWorkload; w != nil && w.Min == w.Max && w.Max == float64(int(w.Max)) {
			n := int(w.Max)
			o.Hours = &n
		}
	}
	if (previous|o.LearningOwnership)&learningReview != 0 {
		o.Review = old.Review != nil && old.Review.Available
	}
	if (previous|o.LearningOwnership)&learningMentor != 0 {
		o.Mentor = old.Mentor != nil && old.Mentor.Available
	}
	if (previous|o.LearningOwnership)&(learningReview|learningMentor) != 0 {
		known := old.Review != nil && old.Mentor != nil
		o.SupportKnown = &known
	}
	if len(old.Evidence()) == 0 {
		o.LearningConditions = nil
	} else {
		o.LearningConditions = &old
	}
}

// A collector must attest the same facts in the normalized record and envelope.
// Evidence from another course or another tariff is never accepted implicitly.
func validLearningObservation(v *catalog.LearningConditions, observed bool, record *catalog.LearningConditions, sources []string, at time.Time) bool {
	if !observed {
		return v == nil && record == nil
	}
	if !v.Valid() {
		return false
	}
	a, _ := json.Marshal(v)
	b, _ := json.Marshal(record)
	if string(a) != string(b) {
		return false
	}
	for _, e := range v.Evidence() {
		allowed := false
		for _, source := range sources {
			if e.Source == source {
				allowed = true
			}
		}
		if !allowed || !e.CheckedAt.UTC().Truncate(time.Microsecond).Equal(at.UTC().Truncate(time.Microsecond)) {
			return false
		}
	}
	return true
}

func learningObservationForOffer(o Observation, id string) Observation {
	if v, ok := o.LearningByOffer[id]; ok {
		o.LearningObserved = true
		o.LearningConditions = v
	}
	return o
}
func validLearningEnvelope(e QueuedObservation) bool {
	o := e.Observation
	if len(o.LearningByOffer) > len(e.Record.Offers) || len(o.LearningByOffer) > 0 && (o.LearningObserved || o.LearningConditions != nil) || o.LearningObserved && len(e.Record.Offers) != 1 {
		return false
	}
	found := 0
	for _, r := range e.Record.Offers {
		v, observed := o.LearningByOffer[r.ID]
		if observed {
			found++
		} else if o.LearningObserved {
			v, observed = o.LearningConditions, true
		}
		sources := []string{e.Candidate.URL, candidateEndpoint(e.Candidate)}
		if e.Candidate.Adapter == "yandex" {
			slug := e.Candidate.ExternalID
			for _, t := range o.PracticumTariffs {
				if r.ID == e.Record.ID+"-"+t.Slug {
					slug = t.Slug
				}
			}
			sources = append(sources, practicumProfessionURL(slug))
		}
		if !validLearningObservation(v, observed, r.LearningConditions, sources, e.ObservedAt) {
			return false
		}
	}
	return found == len(o.LearningByOffer)
}
