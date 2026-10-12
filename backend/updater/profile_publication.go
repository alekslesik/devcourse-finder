package updater

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"time"
)

const (
	profilePrerequisites = 1 << iota
	profileAudience
	profileGoals
	profileDirectionBit
	profileTarget
	profileExclusions
)

func knownProfileTags(v []string) bool {
	for _, s := range v {
		if s != "unknown" && s != "" {
			return true
		}
	}
	return false
}

// Collector ownership is per field. Missing evidence clears collector-owned facts,
// while an explicit operator import (ownership zero) remains authoritative.
func applyCourseProfile(c *catalog.Course, incoming *catalog.CourseProfile) {
	old, next := catalog.CourseProfile{}, catalog.CourseProfile{}
	if c.Profile != nil {
		old = *c.Profile
	}
	if incoming != nil {
		next = *incoming
	}
	prev := c.ProfileOwnership
	owned := func(bit int, present, curated bool) bool { return prev&bit != 0 || !present && !curated }
	setBit := func(bit int, present bool) {
		c.ProfileOwnership &^= bit
		if present {
			c.ProfileOwnership |= bit
		}
	}
	if owned(profilePrerequisites, old.Prerequisites != nil, false) {
		old.Prerequisites = next.Prerequisites
		setBit(profilePrerequisites, old.Prerequisites != nil)
	}
	if owned(profileAudience, old.Audience != nil, knownProfileTags(c.Audience)) {
		old.Audience = next.Audience
		setBit(profileAudience, old.Audience != nil)
	}
	if owned(profileGoals, old.Goals != nil, knownProfileTags(c.Goals)) {
		old.Goals = next.Goals
		setBit(profileGoals, old.Goals != nil)
	}
	if owned(profileDirectionBit, old.Direction != nil, c.Direction != "" && c.Direction != "unknown") {
		old.Direction = next.Direction
		setBit(profileDirectionBit, old.Direction != nil)
	}
	if owned(profileTarget, old.TargetLevel != nil, false) {
		old.TargetLevel = next.TargetLevel
		setBit(profileTarget, old.TargetLevel != nil)
	}
	if (prev|c.ProfileOwnership)&profileAudience != 0 {
		c.Audience = []string{"unknown"}
		if old.Audience != nil {
			c.Audience = append([]string(nil), old.Audience.Values...)
		}
	}
	if (prev|c.ProfileOwnership)&profileGoals != 0 {
		c.Goals = []string{"unknown"}
		if old.Goals != nil {
			c.Goals = append([]string(nil), old.Goals.Values...)
		}
	}
	if (prev|c.ProfileOwnership)&profileDirectionBit != 0 {
		c.Direction = "unknown"
		if old.Direction != nil {
			c.Direction = old.Direction.Value
		}
	}
	if owned(profileExclusions, old.ExcludedAudience != nil, false) {
		old.ExcludedAudience = next.ExcludedAudience
		setBit(profileExclusions, old.ExcludedAudience != nil)
	}
	c.Profile = nil
	if len(old.Evidence()) > 0 {
		c.Profile = &old
	}
}
func applyPublishedProfile(result []catalog.Course, old []catalog.Course, o Observation) {
	if !o.ProfileObserved {
		return
	}
	for i := range result {
		exists := false
		for _, c := range old {
			if c.ID == result[i].ID {
				exists = true
				break
			}
		}
		if !exists {
			result[i].Profile = nil
			result[i].ProfileOwnership = 0
		}
		applyCourseProfile(&result[i], o.Profile)
	}
}
func validProfileEnvelope(e QueuedObservation) bool {
	o := e.Observation
	if !o.ProfileObserved {
		return o.Profile == nil && (e.Kind == "curated" || e.Record.Profile == nil)
	}
	if !o.Profile.Valid() {
		return false
	}
	a, _ := json.Marshal(o.Profile)
	b, _ := json.Marshal(e.Record.Profile)
	if string(a) != string(b) {
		return false
	}
	for _, p := range o.Profile.Evidence() {
		if p.Source != e.Candidate.URL && p.Source != candidateEndpoint(e.Candidate) {
			return false
		}
		if !p.CheckedAt.UTC().Truncate(time.Microsecond).Equal(e.ObservedAt.UTC().Truncate(time.Microsecond)) {
			return false
		}
	}
	return true
}
