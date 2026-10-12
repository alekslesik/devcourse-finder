package catalog

import "slices"

// Profile facts describe the verified course, not the school or a recommendation.
// Missing fields remain unknown. Requirements and audience are independent.
type ProfileValue struct {
	Value    string           `json:"value"`
	Evidence LearningEvidence `json:"evidence"`
}
type ProfileTags struct {
	Values   []string         `json:"values"`
	Evidence LearningEvidence `json:"evidence"`
}
type ProfileRequirement struct {
	Level    string           `json:"level"`
	Scope    string           `json:"scope"`
	Evidence LearningEvidence `json:"evidence"`
}
type CourseProfile struct {
	Prerequisites    *ProfileRequirement `json:"prerequisites,omitempty"`
	ExcludedAudience *ProfileTags        `json:"excluded_audience,omitempty"`
	Audience         *ProfileTags        `json:"audience,omitempty"`
	Goals            *ProfileTags        `json:"goals,omitempty"`
	Direction        *ProfileValue       `json:"direction,omitempty"`
	TargetLevel      *ProfileValue       `json:"target_level,omitempty"`
}

var experienceValues = []string{"none", "basics", "projects", "working", "switch"}
var goalValues = []string{"try", "job", "switch", "deepen"}
var directionValues = []string{"basics", "backend", "frontend", "fullstack", "automation"}

func levelRank(v string) int { return slices.Index(experienceValues[:4], v) }
func (p *CourseProfile) Evidence() []LearningEvidence {
	if p == nil {
		return nil
	}
	var out []LearningEvidence
	if p.Prerequisites != nil {
		out = append(out, p.Prerequisites.Evidence)
	}
	for _, v := range []*ProfileTags{p.Audience, p.Goals, p.ExcludedAudience} {
		if v != nil {
			out = append(out, v.Evidence)
		}
	}
	for _, v := range []*ProfileValue{p.Direction, p.TargetLevel} {
		if v != nil {
			out = append(out, v.Evidence)
		}
	}
	return out
}
func (p *CourseProfile) Valid() bool {
	if p == nil {
		return true
	}
	tags := func(t *ProfileTags, allowed []string) bool {
		if t == nil {
			return true
		}
		if len(t.Values) == 0 || len(t.Values) > len(allowed) {
			return false
		}
		seen := map[string]bool{}
		for _, v := range t.Values {
			if !slices.Contains(allowed, v) || seen[v] {
				return false
			}
			seen[v] = true
		}
		return true
	}
	if !tags(p.Audience, experienceValues) || !tags(p.Goals, goalValues) || !tags(p.ExcludedAudience, experienceValues) {
		return false
	}
	if v := p.Prerequisites; v != nil && (levelRank(v.Level) < 0 || !slices.Contains([]string{"programming", "course_language"}, v.Scope)) {
		return false
	}
	if v := p.TargetLevel; v != nil && levelRank(v.Value) < 0 {
		return false
	}
	if v := p.Direction; v != nil && !slices.Contains(directionValues, v.Value) {
		return false
	}
	if p.Prerequisites != nil && p.TargetLevel != nil && levelRank(p.TargetLevel.Value) < levelRank(p.Prerequisites.Level) {
		return false
	}
	if p.Audience != nil && p.ExcludedAudience != nil {
		for _, v := range p.Audience.Values {
			if slices.Contains(p.ExcludedAudience.Values, v) {
				return false
			}
		}
	}
	if p.Audience != nil && p.Prerequisites != nil {
		for _, v := range p.Audience.Values {
			if rank := levelRank(v); rank >= 0 && rank < levelRank(p.Prerequisites.Level) {
				return false
			}
		}
	}
	for _, e := range p.Evidence() {
		if !e.Valid() {
			return false
		}
	}
	return true
}

// MatchProfile returns confirmed, unknown or incompatible. Existing curated
// tags retain literal semantics; a minimum prerequisite alone is not suitability.
func MatchProfile(c Course, key, value string) string {
	if value == "" {
		return "confirmed"
	}
	p := c.Profile
	var tags []string
	switch key {
	case "experience":
		if value == "experienced" {
			unknown := false
			for _, v := range experienceValues[1:] {
				switch MatchProfile(c, key, v) {
				case "confirmed":
					return "confirmed"
				case "unknown":
					unknown = true
				}
			}
			if unknown {
				return "unknown"
			}
			return "incompatible"
		}
		if p != nil && p.ExcludedAudience != nil && slices.Contains(p.ExcludedAudience.Values, value) {
			return "incompatible"
		}
		if p != nil && p.Prerequisites != nil {
			r := p.Prerequisites
			rank := levelRank(value)
			if value == "switch" {
				if r.Scope == "programming" {
					rank = 1
				} else {
					rank = 0
				}
			}
			if value != "experienced" && rank < levelRank(r.Level) {
				return "incompatible"
			}
		}
		tags = c.Audience
		if p != nil && p.Audience != nil {
			tags = p.Audience.Values
		}
		if matchesExperience(tags, value) {
			return "confirmed"
		}
		known := slices.ContainsFunc(tags, func(v string) bool { return slices.Contains(experienceValues, v) })
		// Only a verified target together with verified prerequisites can establish
		// an interval. A 'from zero' requirement does not match every skilled user.
		if p != nil && p.Prerequisites != nil && p.TargetLevel != nil && value != "switch" && value != "experienced" && (!known || p.Audience != nil) {
			rank := levelRank(value)
			if rank >= levelRank(p.Prerequisites.Level) && rank <= levelRank(p.TargetLevel.Value) {
				return "confirmed"
			}
			return "incompatible"
		}
		if known {
			return "incompatible"
		}
	case "goal":
		tags = c.Goals
		if p != nil && p.Goals != nil {
			tags = p.Goals.Values
		}
		if slices.Contains(tags, value) {
			return "confirmed"
		}
		if slices.ContainsFunc(tags, func(v string) bool { return slices.Contains(goalValues, v) }) {
			return "incompatible"
		}
	case "direction":
		v := c.Direction
		if p != nil && p.Direction != nil {
			v = p.Direction.Value
		}
		if v == value {
			return "confirmed"
		}
		if slices.Contains(directionValues, v) {
			return "incompatible"
		}
	}
	return "unknown"
}
