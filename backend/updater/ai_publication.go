package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"devcourse-finder/catalog"
	"github.com/jackc/pgx/v5"
)

const aiQueueLimit = 1000
const aiPublisherLimit = 40
const aiProfileMask = profilePrerequisites | profileAudience | profileGoals | profileDirectionBit | profileTarget | profileExclusions
const aiLearningMask = learningDuration | learningWorkload | learningReview | learningMentor | learningCommunity

type AIEvent struct {
	Version        int            `json:"version"`
	CourseID       string         `json:"course_id"`
	IdentityDigest string         `json:"identity_digest"`
	Candidate      Candidate      `json:"candidate"`
	ObservedAt     time.Time      `json:"observed_at"`
	Model          string         `json:"model"`
	Policy         string         `json:"policy"`
	Documents      []AIDocument   `json:"documents"`
	Extraction     AIExtraction   `json:"extraction"`
	Verification   AIVerification `json:"verification"`
}

func aiIdentityDigest(c catalog.Course) string {
	type identityOffer struct{ ID, Name, URL string }
	offers := []identityOffer{}
	for _, o := range c.Offers {
		offers = append(offers, identityOffer{o.ID, o.Name, o.URL})
	}
	sort.Slice(offers, func(i, j int) bool { return offers[i].ID < offers[j].ID })
	raw, _ := json.Marshal(struct {
		ID, Title, Language, Source string
		Offers                      []identityOffer
	}{c.ID, c.Title, c.Language, c.Source, offers})
	return fingerprint(raw)
}
func (e AIEvent) valid() bool {
	c, ok := candidateURL(e.Candidate.Adapter, e.Candidate.URL, "")
	if !ok || c.URL != e.Candidate.URL || c.ExternalID != e.Candidate.ExternalID || e.Version != 1 || e.CourseID == "" || !evidenceDigest.MatchString(e.IdentityDigest) || e.Policy != aiEffectivePolicy() || len(e.Model) == 0 || len(e.Model) > 200 || e.ObservedAt.IsZero() {
		return false
	}
	_, err := aiCompile(c, e.Documents, e.Extraction, e.Verification, e.ObservedAt)
	return err == nil
}
func (s *Service) enqueueAI(ctx context.Context, e AIEvent) error {
	if !e.valid() {
		return ErrAI
	}
	raw, _ := json.Marshal(e)
	if len(raw) > 262144 {
		return ErrAI
	}
	tx, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(829117)"); err != nil {
		return err
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM catalog_ai_observations WHERE processed_at IS NULL").Scan(&count); err != nil {
		return err
	}
	if count >= aiQueueLimit {
		return ErrBusy
	}
	if _, err = tx.Exec(ctx, "INSERT INTO catalog_ai_observations(course_id,observed_at,payload) VALUES($1,$2,$3::jsonb) ON CONFLICT(course_id,observed_at) DO NOTHING", e.CourseID, e.ObservedAt, raw); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO catalog_ai_sources(course_id,attempted_at,next_attempt_at,code,failures) VALUES($1,$2,now()+interval '12 hours','queued',0) ON CONFLICT(course_id) DO UPDATE SET code='queued',next_attempt_at=now()+interval '12 hours',failures=0 WHERE catalog_ai_sources.attempted_at<=$2`, e.CourseID, e.ObservedAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func applyAIProfile(c *catalog.Course, incoming *catalog.CourseProfile, partial bool) {
	old, next := catalog.CourseProfile{}, catalog.CourseProfile{}
	if c.Profile != nil {
		old = *c.Profile
	}
	if incoming != nil {
		next = *incoming
	}
	previousOwnership := c.ProfileOwnership
	preserved := 0
	if partial {
		if next.Prerequisites == nil {
			preserved |= profilePrerequisites
		}
		if next.Audience == nil {
			preserved |= profileAudience
		}
		if next.Goals == nil {
			preserved |= profileGoals
		}
		if next.Direction == nil {
			preserved |= profileDirectionBit
		}
		if next.TargetLevel == nil {
			preserved |= profileTarget
		}
		if next.ExcludedAudience == nil {
			preserved |= profileExclusions
		}
	}
	defer func() {
		bits := preserved | (preserved << aiOwnershipShift)
		c.ProfileOwnership = (c.ProfileOwnership &^ bits) | (previousOwnership & bits)
	}()
	previousAI := (c.ProfileOwnership >> aiOwnershipShift) & aiProfileMask
	if next.Prerequisites == nil && (partial || previousAI&profilePrerequisites == 0) {
		next.Prerequisites = old.Prerequisites
	}
	if next.Audience == nil && (partial || previousAI&profileAudience == 0) {
		next.Audience = old.Audience
	}
	if next.Goals == nil && (partial || previousAI&profileGoals == 0) {
		next.Goals = old.Goals
	}
	if next.Direction == nil && (partial || previousAI&profileDirectionBit == 0) {
		next.Direction = old.Direction
	}
	if next.TargetLevel == nil && (partial || previousAI&profileTarget == 0) {
		next.TargetLevel = old.TargetLevel
	}
	if next.ExcludedAudience == nil && (partial || previousAI&profileExclusions == 0) {
		next.ExcludedAudience = old.ExcludedAudience
	}
	c.ProfileOwnership &^= aiProfileMask << aiOwnershipShift
	applyCourseProfile(c, &next)
	if c.Profile == nil || incoming == nil {
		return
	}
	mark := func(bit int, accepted bool) {
		if accepted && c.ProfileOwnership&bit != 0 {
			c.ProfileOwnership |= bit << aiOwnershipShift
		}
	}
	mark(profilePrerequisites, incoming.Prerequisites != nil && c.Profile.Prerequisites == incoming.Prerequisites)
	mark(profileAudience, incoming.Audience != nil && c.Profile.Audience == incoming.Audience)
	mark(profileGoals, incoming.Goals != nil && c.Profile.Goals == incoming.Goals)
	mark(profileDirectionBit, incoming.Direction != nil && c.Profile.Direction == incoming.Direction)
	mark(profileTarget, incoming.TargetLevel != nil && c.Profile.TargetLevel == incoming.TargetLevel)
	mark(profileExclusions, incoming.ExcludedAudience != nil && c.Profile.ExcludedAudience == incoming.ExcludedAudience)
}
func applyAILearning(o *catalog.Offer, incoming *catalog.LearningConditions, partial bool) {
	old, next := catalog.LearningConditions{}, catalog.LearningConditions{}
	if o.LearningConditions != nil {
		old = *o.LearningConditions
	}
	if incoming != nil {
		next = *incoming
	}
	previousOwnership := o.LearningOwnership
	preserved := 0
	if partial {
		if next.Duration == nil {
			preserved |= learningDuration
		}
		if next.WeeklyWorkload == nil {
			preserved |= learningWorkload
		}
		if next.Review == nil {
			preserved |= learningReview
		}
		if next.Mentor == nil {
			preserved |= learningMentor
		}
		if next.Community == nil {
			preserved |= learningCommunity
		}
	}
	defer func() {
		bits := preserved | (preserved << aiOwnershipShift)
		o.LearningOwnership = (o.LearningOwnership &^ bits) | (previousOwnership & bits)
	}()
	previousAI := (o.LearningOwnership >> aiOwnershipShift) & aiLearningMask
	if next.Duration == nil && (partial || previousAI&learningDuration == 0) {
		next.Duration = old.Duration
	}
	if next.WeeklyWorkload == nil && (partial || previousAI&learningWorkload == 0) {
		next.WeeklyWorkload = old.WeeklyWorkload
	}
	if next.Review == nil && (partial || previousAI&learningReview == 0) {
		next.Review = old.Review
	}
	if next.Mentor == nil && (partial || previousAI&learningMentor == 0) {
		next.Mentor = old.Mentor
	}
	if next.Community == nil && (partial || previousAI&learningCommunity == 0) {
		next.Community = old.Community
	}
	o.LearningOwnership &^= aiLearningMask << aiOwnershipShift
	applyLearningConditions(o, &next)
	if o.LearningConditions == nil || incoming == nil {
		return
	}
	mark := func(bit int, accepted bool) {
		if accepted && o.LearningOwnership&bit != 0 {
			o.LearningOwnership |= bit << aiOwnershipShift
		}
	}
	mark(learningDuration, incoming.Duration != nil && o.LearningConditions.Duration == incoming.Duration)
	mark(learningWorkload, incoming.WeeklyWorkload != nil && o.LearningConditions.WeeklyWorkload == incoming.WeeklyWorkload)
	mark(learningReview, incoming.Review != nil && o.LearningConditions.Review == incoming.Review)
	mark(learningMentor, incoming.Mentor != nil && o.LearningConditions.Mentor == incoming.Mentor)
	mark(learningCommunity, incoming.Community != nil && o.LearningConditions.Community == incoming.Community)
}
func mergeAI(old []catalog.Course, e AIEvent, code *string) ([]catalog.Course, error) {
	facts, err := aiCompile(e.Candidate, e.Documents, e.Extraction, e.Verification, e.ObservedAt)
	if err != nil {
		*code = "invalid_ai_observation"
		return nil, nil
	}
	for _, current := range old {
		if current.ID != e.CourseID {
			continue
		}
		if current.Status != "published" || current.Demo || current.Source != e.Candidate.URL || aiIdentityDigest(current) != e.IdentityDigest {
			*code = "protected_ai_identity"
			return nil, nil
		}
		c := current
		c.Offers = append([]catalog.Offer(nil), current.Offers...)
		for _, d := range e.Documents {
			if d.Scope == "offer" {
				found := false
				for _, o := range c.Offers {
					if o.ID == d.OfferID && o.URL == c.Source {
						found = true
					}
				}
				if !found {
					*code = "protected_ai_identity"
					return nil, nil
				}
			}
		}
		applyAIProfile(&c, facts.Profile, e.Extraction.DiscardedClaims > 0)
		for i := range c.Offers {
			if incoming, observed := facts.Learning[c.Offers[i].ID]; observed {
				applyAILearning(&c.Offers[i], incoming, e.Extraction.DiscardedClaims > 0)
			}
		}
		if !c.Profile.Valid() {
			*code = "conflicting_ai_facts"
			return nil, nil
		}
		before, _ := json.Marshal(current)
		after, _ := json.Marshal(c)
		same := bytes.Equal(before, after) && current.ProfileOwnership == c.ProfileOwnership
		for i := range c.Offers {
			same = same && current.Offers[i].LearningOwnership == c.Offers[i].LearningOwnership
		}
		if same {
			*code = "ai_no_change"
			return nil, nil
		}
		*code = "ai_verified"
		return []catalog.Course{c}, nil // No price, enrollment or whole-course dates change.
	}
	*code = "protected_ai_identity"
	return nil, nil
}
func (s *Service) publishAI(ctx context.Context) (int, error) {
	updated := 0
	for i := 0; i < aiPublisherLimit; i++ {
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 5*time.Second {
			break
		}
		var id int64
		var courseID string
		var at time.Time
		var raw []byte
		err := s.DB.Pool.QueryRow(ctx, "SELECT id,course_id,observed_at,payload FROM catalog_ai_observations WHERE processed_at IS NULL ORDER BY id LIMIT 1").Scan(&id, &courseID, &at, &raw)
		if errors.Is(err, pgx.ErrNoRows) {
			break
		}
		if err != nil {
			return updated, err
		}
		var e AIEvent
		valid := aiStrictJSON(raw, &e) == nil && e.valid() && e.CourseID == courseID && e.ObservedAt.Equal(at)
		code := "invalid_ai_observation"
		n, err := s.DB.UpdateVerified(ctx, []string{"stepik.org", "practicum.yandex.ru"}, "automatic-ai-catalog-publisher", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
			var courses []catalog.Course
			if valid {
				var last *time.Time
				err := tx.QueryRow(ctx, "SELECT observed_at FROM catalog_ai_watermarks WHERE course_id=$1", courseID).Scan(&last)
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return nil, err
				}
				stale := time.Since(at) > 48*time.Hour || at.After(time.Now().Add(time.Minute)) || last != nil && !at.After(*last)
				if stale {
					code = "stale_ai_observation"
				} else {
					courses, err = mergeAI(old, e, &code)
					if err != nil {
						return nil, err
					}
					if _, err = tx.Exec(ctx, "INSERT INTO catalog_ai_watermarks(course_id,observed_at,event_id) VALUES($1,$2,$3) ON CONFLICT(course_id) DO UPDATE SET observed_at=EXCLUDED.observed_at,event_id=EXCLUDED.event_id", courseID, at, id); err != nil {
						return nil, err
					}
				}
			}
			if _, err := tx.Exec(ctx, "UPDATE catalog_ai_observations SET processed_at=now(),code=$2 WHERE id=$1 AND processed_at IS NULL", id, code); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, `UPDATE catalog_ai_sources SET code=$2,verified_at=CASE WHEN $2 IN ('ai_verified','ai_no_change') THEN $3 ELSE verified_at END WHERE course_id=$1 AND attempted_at<=$3`, courseID, code, at); err != nil {
				return nil, err
			}
			return courses, nil
		})
		if err != nil {
			return updated, err
		}
		updated += n
	}
	_, err := s.DB.Pool.Exec(ctx, `DELETE FROM catalog_ai_observations WHERE id IN (SELECT id FROM catalog_ai_observations WHERE processed_at IS NOT NULL AND processed_at<now()-interval '7 days' ORDER BY id LIMIT 1000)`)
	return updated, err
}
