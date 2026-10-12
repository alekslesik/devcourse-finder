package updater

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"devcourse-finder/catalog"
)

const aiPolicyVersion = "course-facts-v1"
const aiMaxText = 48000
const aiMaxClaims = 40
const aiOwnershipShift = 8

var ErrAI = errors.New("AI observation could not be verified")

// Documents contain only bounded public course text, never cookies or secrets.
// Sources and scopes come from the fetcher, not from model-generated URLs.
type AIDocument struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Scope   string `json:"scope"`
	OfferID string `json:"offer_id"`
	Text    string `json:"text"`
	Digest  string `json:"digest"`
}
type AIClaim struct {
	Field      string   `json:"field"`
	OfferID    string   `json:"offer_id"`
	DocumentID string   `json:"document_id"`
	Quote      string   `json:"quote"`
	Value      string   `json:"value"`
	Values     []string `json:"values"`
	Scope      string   `json:"scope"`
	Min        *float64 `json:"min"`
	Max        *float64 `json:"max"`
	Unit       string   `json:"unit"`
	Available  *bool    `json:"available"`
}
type AIExtraction struct {
	CourseURL string    `json:"course_url"`
	Complete  bool      `json:"complete"`
	Claims    []AIClaim `json:"claims"`
}
type AIVerification struct {
	Complete bool  `json:"complete"`
	Verified []int `json:"verified_claim_ids"`
}
type AIFacts struct {
	Profile  *catalog.CourseProfile
	Learning map[string]*catalog.LearningConditions
}

func aiStrictJSON(raw []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrAI
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrAI
	}
	return nil
}
func aiNormalize(s string) string { return strings.Join(strings.Fields(s), " ") }
func aiNumberInQuote(n float64, q string) bool {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return false
	}
	// Compare whole numeric tokens, never accept 1 as a rounded version of 1.5.
	for _, token := range regexp.MustCompile(`[0-9]+(?:[.,][0-9]+)?`).FindAllString(q, -1) {
		value, err := strconv.ParseFloat(strings.ReplaceAll(token, ",", "."), 64)
		if err == nil && value == n {
			return true
		}
	}
	return false
}

func aiDocumentsValid(c Candidate, docs []AIDocument) bool {
	if c.Adapter != "stepik" && c.Adapter != "yandex" || len(docs) == 0 || len(docs) > 9 {
		return false
	}
	seen := map[string]bool{}
	total := 0
	courseCount := 0
	for _, d := range docs {
		if d.ID == "" || seen[d.ID] || d.Text == "" || !evidenceDigest.MatchString(d.Digest) {
			return false
		}
		seen[d.ID] = true
		total += len(d.Text)
		if total > aiMaxText {
			return false
		}
		if d.Scope == "course" {
			courseCount++
			if d.OfferID != "" || d.Source != c.URL && d.Source != candidateEndpoint(c) {
				return false
			}
		} else if d.Scope == "offer" {
			if d.OfferID == "" || d.Source != c.URL && d.Source != candidateEndpoint(c) {
				return false
			}
		} else {
			return false
		}
	}
	return courseCount == 1
}
func aiCompile(c Candidate, docs []AIDocument, extraction AIExtraction, verification AIVerification, at time.Time) (AIFacts, error) {
	out := AIFacts{Profile: &catalog.CourseProfile{}, Learning: map[string]*catalog.LearningConditions{}}
	if !aiDocumentsValid(c, docs) || extraction.CourseURL != c.URL || !extraction.Complete || !verification.Complete || len(extraction.Claims) > aiMaxClaims {
		return out, ErrAI
	}
	documents := map[string]AIDocument{}
	for _, d := range docs {
		documents[d.ID] = d
	}
	accepted := map[int]bool{}
	for _, i := range verification.Verified {
		if i < 0 || i >= len(extraction.Claims) || accepted[i] {
			return out, ErrAI
		}
		accepted[i] = true
	}
	seen := map[string]bool{}
	for i, claim := range extraction.Claims {
		d, ok := documents[claim.DocumentID]
		key := claim.OfferID + ":" + claim.Field
		q := aiNormalize(claim.Quote)
		if !ok || seen[key] || len(q) < 12 || len(q) > 1200 || !strings.Contains(aiNormalize(d.Text), q) {
			return out, ErrAI
		}
		seen[key] = true
		course := slices.Contains([]string{"prerequisites", "audience", "excluded_audience", "goals", "direction", "target_level"}, claim.Field)
		learning := slices.Contains([]string{"duration", "weekly_workload", "review", "mentor", "community"}, claim.Field)
		if course && (claim.OfferID != "" || d.Scope != "course") || learning && (claim.OfferID == "" || d.Scope != "offer" || d.OfferID != claim.OfferID) || !course && !learning {
			return out, ErrAI
		}
		evidence := catalog.LearningEvidence{Source: d.Source, Locator: "ai:" + claim.Field + ";quote_sha256=" + fingerprint([]byte(q)), Digest: d.Digest, CheckedAt: at}
		p := catalog.CourseProfile{}
		conditions := catalog.LearningConditions{}
		numeric := claim.Field == "duration" || claim.Field == "weekly_workload"
		support := claim.Field == "review" || claim.Field == "mentor" || claim.Field == "community"
		tags := claim.Field == "audience" || claim.Field == "excluded_audience" || claim.Field == "goals"
		if !numeric && (claim.Min != nil || claim.Max != nil || claim.Unit != "") || !support && claim.Available != nil || !tags && len(claim.Values) > 0 || claim.Field != "prerequisites" && claim.Scope != "" {
			return out, ErrAI
		}
		if (numeric || support || tags) && claim.Value != "" {
			return out, ErrAI
		}
		switch claim.Field {
		case "prerequisites":
			p.Prerequisites = &catalog.ProfileRequirement{Level: claim.Value, Scope: claim.Scope, Evidence: evidence}
		case "audience":
			p.Audience = &catalog.ProfileTags{Values: claim.Values, Evidence: evidence}
		case "excluded_audience":
			p.ExcludedAudience = &catalog.ProfileTags{Values: claim.Values, Evidence: evidence}
		case "goals":
			p.Goals = &catalog.ProfileTags{Values: claim.Values, Evidence: evidence}
		case "direction":
			p.Direction = &catalog.ProfileValue{Value: claim.Value, Evidence: evidence}
		case "target_level":
			p.TargetLevel = &catalog.ProfileValue{Value: claim.Value, Evidence: evidence}
		case "duration", "weekly_workload":
			if claim.Min == nil || claim.Max == nil || !aiNumberInQuote(*claim.Min, q) || !aiNumberInQuote(*claim.Max, q) {
				return out, ErrAI
			}
			r := &catalog.LearningRange{Min: *claim.Min, Max: *claim.Max, Unit: claim.Unit, Evidence: evidence}
			if claim.Field == "duration" {
				conditions.Duration = r
			} else {
				conditions.WeeklyWorkload = r
			}
		case "review", "mentor", "community":
			if claim.Available == nil {
				return out, ErrAI
			}
			s := &catalog.LearningSupport{Available: *claim.Available, Evidence: evidence}
			switch claim.Field {
			case "review":
				conditions.Review = s
			case "mentor":
				conditions.Mentor = s
			case "community":
				conditions.Community = s
			}
		}
		if !p.Valid() || !conditions.Valid() {
			return out, ErrAI
		}
		if !accepted[i] {
			continue
		}
		switch claim.Field {
		case "prerequisites":
			out.Profile.Prerequisites = p.Prerequisites
		case "audience":
			out.Profile.Audience = p.Audience
		case "excluded_audience":
			out.Profile.ExcludedAudience = p.ExcludedAudience
		case "goals":
			out.Profile.Goals = p.Goals
		case "direction":
			out.Profile.Direction = p.Direction
		case "target_level":
			out.Profile.TargetLevel = p.TargetLevel
		default:
			old := out.Learning[claim.OfferID]
			if old == nil {
				old = &catalog.LearningConditions{}
			}
			switch claim.Field {
			case "duration":
				old.Duration = conditions.Duration
			case "weekly_workload":
				old.WeeklyWorkload = conditions.WeeklyWorkload
			case "review":
				old.Review = conditions.Review
			case "mentor":
				old.Mentor = conditions.Mentor
			case "community":
				old.Community = conditions.Community
			}
			out.Learning[claim.OfferID] = old
		}
	}
	if !out.Profile.Valid() {
		return out, ErrAI
	}
	if len(out.Profile.Evidence()) == 0 {
		out.Profile = nil
	}
	// Empty successful observations still attest disappearance of AI-owned fields.
	for _, d := range docs {
		if d.Scope == "offer" {
			if _, ok := out.Learning[d.OfferID]; !ok {
				out.Learning[d.OfferID] = nil
			}
		}
	}
	return out, nil
}
