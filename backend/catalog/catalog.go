package catalog

import (
	"sort"
	"time"
)

type Course struct {
	Profile          *CourseProfile `json:"profile,omitempty"`
	ProfileOwnership int            `json:"-"`
	ID               string         `json:"id"`
	Slug             string         `json:"slug"`
	Title            string         `json:"title"`
	Provider         string         `json:"provider"`
	Language         string         `json:"language"`
	Direction        string         `json:"direction"`
	Summary          string         `json:"summary"`
	Audience         []string       `json:"audience"`
	Goals            []string       `json:"goals"`
	Topics           []string       `json:"topics"`
	Source           string         `json:"source"`
	CheckedAt        time.Time      `json:"checked_at"`
	Status           string         `json:"status"`
	Offers           []Offer        `json:"offers"`
	Demo             bool           `json:"demo"`
}

// Billing preserves a recurring fee in its original currency. It is never a
// verified complete-course RUB price and cannot satisfy a price budget.
type Billing struct {
	Kind        string `json:"kind"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
	Interval    string `json:"interval"`
}

func (b *Billing) Valid() bool {
	return b != nil && b.Kind == "subscription" && b.AmountMinor > 0 && b.AmountMinor <= 1000000000 && (b.Currency == "USD" || b.Currency == "EUR" || b.Currency == "RUB") && (b.Interval == "month" || b.Interval == "year")
}

type Offer struct {
	LearningConditions *LearningConditions `json:"learning_conditions,omitempty"`
	LearningOwnership  int                 `json:"-"`
	DurationCollected  bool                `json:"-"`
	DurationMonths     *int                `json:"duration_months,omitempty"`
	Billing            *Billing            `json:"billing,omitempty"`
	SupportKnown       *bool               `json:"support_known,omitempty"`
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	Price              *int64              `json:"price"`
	PriceKind          string              `json:"price_kind"`
	Free               bool                `json:"free"`
	PriceCheckedAt     time.Time           `json:"price_checked_at"`
	ValidUntil         *time.Time          `json:"valid_until,omitempty"`
	Hours              *int                `json:"hours"`
	Weeks              *int                `json:"weeks"`
	Review             bool                `json:"review"`
	Mentor             bool                `json:"mentor"`
	Schedule           string              `json:"schedule"`
	Enrollment         string              `json:"enrollment"`
	URL                string              `json:"url"`
}
type Filter struct {
	Language, Direction, Experience, Goal, Budget, Support, Schedule, Sort string
	Min, Max                                                               *int64
	Hours                                                                  *int
	IncludeFree, IncludeClosed, Diagnostics, IncludeUnknownProfiles        bool
	Page, PageSize                                                         int
}
type Result struct {
	ProfileMatch         string   `json:"profile_match,omitempty"`
	UnknownProfileFields []string `json:"unknown_profile_fields,omitempty"`
	Course               Course   `json:"course"`
	Offer                Offer    `json:"offer"`
	Price                *int64   `json:"effective_price"`
	Reasons              []string `json:"reasons"`
	Stale                bool     `json:"stale"`
}

func EffectivePrice(o Offer, now time.Time) *int64 {
	if o.Billing != nil || o.Price == nil || o.PriceKind != "exact" || o.PriceCheckedAt.IsZero() || now.Sub(o.PriceCheckedAt) > 30*24*time.Hour || (o.ValidUntil != nil && !now.Before(*o.ValidUntil)) {
		return nil
	}
	return o.Price
}
func Search(courses []Course, f Filter, now time.Time) []Result {
	out := []Result{}
	hasPriceBounds := f.Min != nil && *f.Min > 0 || f.Max != nil
	for _, c := range courses {
		if c.Status != "published" || (f.Language != "" && c.Language != f.Language) {
			continue
		}
		unknown := []string{}
		incompatible := false
		for _, pair := range [][2]string{{"experience", f.Experience}, {"goal", f.Goal}, {"direction", f.Direction}} {
			switch MatchProfile(c, pair[0], pair[1]) {
			case "incompatible":
				incompatible = true
			case "unknown":
				unknown = append(unknown, pair[0])
			}
		}
		if incompatible || len(unknown) > 0 && !f.IncludeUnknownProfiles {
			continue
		}
		match := "confirmed"
		if len(unknown) > 0 {
			match = "unverified"
		}
		candidates := []Result{}
		for _, o := range c.Offers {
			if !f.IncludeClosed && o.Enrollment != "open" && o.Enrollment != "continuous" {
				continue
			}
			price := EffectivePrice(o, now)
			if f.Budget == "free" && (!o.Free || price == nil || *price != 0) {
				continue
			}
			if f.Budget == "paid" && o.Free && !f.IncludeFree {
				continue
			}
			if hasPriceBounds {
				if !(f.IncludeFree && o.Free && price != nil && *price == 0) {
					if price == nil || (f.Min != nil && *price < *f.Min) || (f.Max != nil && *price > *f.Max) {
						continue
					}
				}
			}
			if f.Hours != nil && !o.WithinWeeklyHours(*f.Hours) {
				continue
			}
			if !o.MatchesSupport(f.Support) {
				continue
			}
			if f.Schedule != "" && o.Schedule != f.Schedule {
				continue
			}
			reasons := []string{}
			if f.Goal != "" && MatchProfile(c, "goal", f.Goal) == "confirmed" {
				reasons = append(reasons, "Соответствует вашей цели")
			}
			if f.Experience != "" && MatchProfile(c, "experience", f.Experience) == "confirmed" {
				reasons = append(reasons, "Подходит вашему опыту")
			}
			if hasPriceBounds {
				reasons = append(reasons, "В пределах бюджета")
			}
			if f.Support == "review" {
				reasons = append(reasons, "Проверка кода человеком")
			}
			if f.Hours != nil {
				reasons = append(reasons, "Подходит по нагрузке")
			}
			if len(reasons) > 3 {
				reasons = reasons[:3]
			}
			candidates = append(candidates, Result{ProfileMatch: match, UnknownProfileFields: unknown, Course: c, Offer: o, Price: price, Reasons: reasons, Stale: now.Sub(c.CheckedAt) > 90*24*time.Hour})
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			a, b := candidates[i], candidates[j]
			if a.Price == nil && b.Price != nil {
				return false
			}
			if a.Price != nil && b.Price == nil {
				return true
			}
			if a.Price != nil && b.Price != nil && *a.Price != *b.Price {
				return *a.Price < *b.Price
			}
			return a.Offer.ID < b.Offer.ID
		})
		if len(candidates) > 0 {
			out = append(out, candidates[0])
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.ProfileMatch != b.ProfileMatch {
			return a.ProfileMatch == "confirmed"
		}
		if f.Sort == "price_asc" || f.Sort == "price_desc" {
			if a.Price == nil && b.Price != nil {
				return false
			}
			if a.Price != nil && b.Price == nil {
				return true
			}
			if a.Price != nil && b.Price != nil && *a.Price != *b.Price {
				if f.Sort == "price_desc" {
					return *a.Price > *b.Price
				}
				return *a.Price < *b.Price
			}
		} else if f.Sort == "duration" {
			aDuration, bDuration := durationDays(a.Offer), durationDays(b.Offer)
			if aDuration == 0 && bDuration != 0 {
				return false
			}
			if aDuration != 0 && bDuration == 0 {
				return true
			}
			if aDuration != bDuration {
				return aDuration < bDuration
			}
		} else {
			if a.Stale != b.Stale {
				return !a.Stale
			}
			if !a.Course.CheckedAt.Equal(b.Course.CheckedAt) {
				return a.Course.CheckedAt.After(b.Course.CheckedAt)
			}
		}
		return a.Course.ID < b.Course.ID
	})
	return out
}

// Months remain months in public data; the average month is used only for sorting.
func durationDays(o Offer) float64 {
	if o.LearningConditions != nil && o.LearningConditions.Duration != nil {
		d := o.LearningConditions.Duration
		switch d.Unit {
		case "day":
			return d.Max
		case "week":
			return d.Max * 7
		case "month":
			return d.Max * 365.25 / 12
		case "year":
			return d.Max * 365.25
		}
	}
	if o.DurationMonths != nil {
		return float64(*o.DurationMonths) * 365.25 / 12
	}
	if o.Weeks != nil {
		return float64(*o.Weeks) * 7
	}
	return 0
}
