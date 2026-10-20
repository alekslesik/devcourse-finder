package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"errors"
	"time"
)

type AIVerificationReport struct {
	CourseID           string           `json:"course_id"`
	Run                AIRunResult      `json:"run"`
	Code               string           `json:"code"`
	Model              string           `json:"model,omitempty"`
	Documents          []map[string]any `json:"documents,omitempty"`
	PublishedUpdates   int              `json:"published_updates"`
	FinancialUnchanged bool             `json:"financial_unchanged"`
	FinancialBefore    string           `json:"financial_before,omitempty"`
	FinancialAfter     string           `json:"financial_after,omitempty"`
	VerifiedClaims     []AIClaim        `json:"verified_claims,omitempty"`
}

// Retain every non-AI field, including all identity, billing, enrollment and
// financial timestamps. The digest is an acceptance assertion, not write access.
func aiProtectedDigest(c catalog.Course) string {
	c.Profile = nil
	c.ProfileOwnership = 0
	c.Audience = nil
	c.Goals = nil
	c.Direction = ""
	c.Offers = append([]catalog.Offer(nil), c.Offers...)
	for i := range c.Offers {
		o := &c.Offers[i]
		o.LearningConditions = nil
		o.LearningOwnership = 0
		o.DurationCollected = false
		o.DurationMonths = nil
		o.Hours = nil
		o.Weeks = nil
		o.SupportKnown = nil
		o.Review = false
		o.Mentor = false
	}
	raw, _ := json.Marshal(c)
	return fingerprint(raw)
}
func (s AIService) VerifyCourse(ctx context.Context, id string) (report AIVerificationReport, err error) {
	report.CourseID = id
	s.TargetCourseID = id
	s.Config.BatchSize = 1
	// Same publication locks as the regular publisher; no native observation or
	// another course's pending event is processed by this targeted check.
	lock, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		return report, err
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		lock.Rollback(clean)
	}()
	for _, key := range []int{829105, 829108} {
		query := `SELECT pg_try_advisory_xact_lock($1)`
		if key == 829105 {
			query = `SELECT pg_try_advisory_xact_lock_shared($1)`
		}
		acquired := false
		for attempt := 0; attempt < 30; attempt++ {
			if err = lock.QueryRow(ctx, query, key).Scan(&acquired); err != nil {
				return report, err
			}
			if acquired {
				break
			}
			select {
			case <-ctx.Done():
				return report, ctx.Err()
			case <-time.After(time.Second):
			}
		}
		if !acquired {
			return report, ErrBusy
		}
	}
	load := func() (catalog.Course, error) {
		courses, e := s.DB.Load(ctx)
		if e != nil {
			return catalog.Course{}, e
		}
		for _, c := range courses {
			if c.ID == id {
				return c, nil
			}
		}
		return catalog.Course{}, ErrAI
	}
	before, err := load()
	if err != nil {
		return report, err
	}
	report.FinancialBefore = aiProtectedDigest(before)
	report.Run, err = s.Run(ctx)
	if err != nil {
		return report, err
	}
	if report.Run.Queued != 1 || report.Run.ObservationID == 0 {
		_ = s.DB.Pool.QueryRow(ctx, `SELECT code FROM catalog_ai_sources WHERE course_id=$1`, id).Scan(&report.Code)
		return report, ErrAI
	}

	publisher := Service{DB: s.DB}
	report.PublishedUpdates, err = publisher.publishAISelection(ctx, id, report.Run.ObservationID)
	if err != nil {
		return report, err
	}
	after, err := load()
	if err != nil {
		return report, err
	}
	report.FinancialAfter = aiProtectedDigest(after)
	report.FinancialUnchanged = report.FinancialBefore == report.FinancialAfter
	var raw []byte
	if err = s.DB.Pool.QueryRow(ctx, `SELECT code,payload FROM catalog_ai_observations WHERE id=$1 AND processed_at IS NOT NULL`, report.Run.ObservationID).Scan(&report.Code, &raw); err != nil {
		return report, err
	}
	var e AIEvent
	if json.Unmarshal(raw, &e) != nil {
		return report, ErrAI
	}
	report.Model = e.Model
	for _, d := range e.Documents {
		report.Documents = append(report.Documents, map[string]any{"scope": d.Scope, "offer_id": d.OfferID, "bytes": len(d.Text), "partial": d.Partial, "digest": d.Digest})
	}
	for _, index := range e.Verification.Verified {
		if index < 0 || index >= len(e.Extraction.Claims) {
			return report, ErrAI
		}
		report.VerifiedClaims = append(report.VerifiedClaims, e.Extraction.Claims[index])
	}
	if !report.FinancialUnchanged {
		return report, errors.New("protected financial fields changed")
	}
	if report.Code != "ai_verified" && report.Code != "ai_no_change" || len(report.VerifiedClaims) == 0 {
		return report, ErrAI
	}
	return report, nil
}
