package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// Each fact carries its own confirmation time. Unknown facts are omitted, never
// encoded as false or zero. A range is not collapsed into an exact amount.
type LearningEvidence struct {
	Source    string    `json:"source"`
	Locator   string    `json:"locator"`
	Digest    string    `json:"digest"`
	CheckedAt time.Time `json:"checked_at"`
}
type LearningRange struct {
	Min      float64          `json:"min"`
	Max      float64          `json:"max"`
	Unit     string           `json:"unit"`
	Evidence LearningEvidence `json:"evidence"`
}
type LearningSupport struct {
	Available bool             `json:"available"`
	Evidence  LearningEvidence `json:"evidence"`
}
type LearningConditions struct {
	Duration       *LearningRange   `json:"duration,omitempty"`
	WeeklyWorkload *LearningRange   `json:"weekly_workload,omitempty"`
	Review         *LearningSupport `json:"review,omitempty"`
	Mentor         *LearningSupport `json:"mentor,omitempty"`
	Community      *LearningSupport `json:"community,omitempty"`
}

var learningDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)

func (e LearningEvidence) Valid() bool {
	u, err := url.Parse(e.Source)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Port() == "" && u.Fragment == "" && len(e.Source) <= 2048 && strings.TrimSpace(e.Locator) != "" && len(e.Locator) <= 256 && learningDigest.MatchString(e.Digest) && !e.CheckedAt.IsZero() && !e.CheckedAt.After(time.Now().Add(time.Minute))
}
func (r *LearningRange) Valid(workload bool) bool {
	if r == nil {
		return true
	}
	if math.IsNaN(r.Min) || math.IsInf(r.Min, 0) || math.IsNaN(r.Max) || math.IsInf(r.Max, 0) || r.Min <= 0 || r.Max < r.Min || !r.Evidence.Valid() {
		return false
	}
	if workload {
		return r.Unit == "hours_per_week" && r.Max <= 168
	}
	switch r.Unit {
	case "day":
		return r.Max <= 3660
	case "week":
		return r.Max <= 520
	case "month":
		return r.Max <= 120
	case "year":
		return r.Max <= 10
	}
	return false
}
func (c *LearningConditions) Evidence() []LearningEvidence {
	if c == nil {
		return nil
	}
	var out []LearningEvidence
	for _, r := range []*LearningRange{c.Duration, c.WeeklyWorkload} {
		if r != nil {
			out = append(out, r.Evidence)
		}
	}
	for _, s := range []*LearningSupport{c.Review, c.Mentor, c.Community} {
		if s != nil {
			out = append(out, s.Evidence)
		}
	}
	return out
}
func (c *LearningConditions) Valid() bool {
	if c == nil {
		return true
	}
	if !c.Duration.Valid(false) || !c.WeeklyWorkload.Valid(true) {
		return false
	}
	for _, e := range c.Evidence() {
		if !e.Valid() {
			return false
		}
	}
	return true
}
func (o Offer) WithinWeeklyHours(limit int) bool {
	if o.LearningConditions != nil && o.LearningConditions.WeeklyWorkload != nil {
		return o.LearningConditions.WeeklyWorkload.Max <= float64(limit)
	}
	return o.Hours != nil && *o.Hours <= limit
}
func (o Offer) MatchesSupport(kind string) bool {
	if kind == "" {
		return true
	}
	if c := o.LearningConditions; c != nil {
		switch kind {
		case "review":
			if c.Review != nil {
				return c.Review.Available
			}
		case "mentor":
			if c.Mentor != nil {
				return c.Mentor.Available
			}
		case "self":
			if c.Review != nil || c.Mentor != nil {
				return c.Review != nil && c.Mentor != nil && !c.Review.Available && !c.Mentor.Available
			}
		}
	}
	if o.SupportKnown != nil && !*o.SupportKnown {
		return false
	}
	return kind == "review" && o.Review || kind == "mentor" && o.Mentor || kind == "self" && !o.Review && !o.Mentor
}

// A missing available field is unknown, not an explicitly false source fact.
func (s *LearningSupport) UnmarshalJSON(data []byte) error {
	var raw struct {
		Available *bool            `json:"available"`
		Evidence  LearningEvidence `json:"evidence"`
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&raw); err != nil {
		return err
	}
	if raw.Available == nil {
		return fmt.Errorf("support availability must be explicit")
	}
	*s = LearningSupport{Available: *raw.Available, Evidence: raw.Evidence}
	return nil
}
