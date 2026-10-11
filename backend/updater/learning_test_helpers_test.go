package updater

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"testing"
	"time"
)

// Synthetic second reads must carry their own fact observation times; copying
// yesterday's evidence unchanged into a fresh envelope is intentionally invalid.
func reobserveLearning(t *testing.T, e QueuedObservation, at time.Time) QueuedObservation {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var out QueuedObservation
	if err = json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	update := func(v *catalog.LearningConditions) {
		if v == nil {
			return
		}
		if v.Duration != nil {
			v.Duration.Evidence.CheckedAt = at
		}
		if v.WeeklyWorkload != nil {
			v.WeeklyWorkload.Evidence.CheckedAt = at
		}
		for _, s := range []*catalog.LearningSupport{v.Review, v.Mentor, v.Community} {
			if s != nil {
				s.Evidence.CheckedAt = at
			}
		}
	}
	update(out.Observation.LearningConditions)
	for _, v := range out.Observation.LearningByOffer {
		update(v)
	}
	for i := range out.Record.Offers {
		update(out.Record.Offers[i].LearningConditions)
	}
	return out
}
