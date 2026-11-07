package updater

import (
	"context"
	"devcourse-finder/catalog"
	"time"
)

type AIPreviewReport struct {
	RecommendationsOnly bool                           `json:"recommendations_only"`
	CourseID            string                         `json:"course_id"`
	Policy              string                         `json:"policy"`
	Model               string                         `json:"model,omitempty"`
	Cached              bool                           `json:"cached"`
	Tokens              int                            `json:"tokens"`
	CatalogWrites       int                            `json:"catalog_writes"`
	Recommendations     *catalog.CourseRecommendations `json:"recommendations,omitempty"`
	VerifiedClaims      []AIClaim                      `json:"verified_claims,omitempty"`
	DiscardedClaims     int                            `json:"discarded_claims"`
}

// Preview uses the real source, models, cache and shared daily reservation cap,
// but never enqueues/publishes observations or changes course attempt dates.
func (s AIService) PreviewCourse(ctx context.Context, id string) (report AIPreviewReport, err error) {
	report.RecommendationsOnly = s.Config.RecommendationsOnly
	report.CourseID = id
	report.Policy = aiEffectivePolicy()
	ids, e := ParseAICourseIDs(id)
	if e != nil || len(ids) != 1 {
		return report, ErrAI
	}
	if s.Config.DailyLimit < 1 || s.Config.DailyLimit > 200 {
		return report, ErrAI
	}
	lock, e := s.DB.Pool.Begin(ctx)
	if e != nil {
		return report, e
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		lock.Rollback(clean)
	}()
	var acquired bool
	if e = lock.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(829116)").Scan(&acquired); e != nil {
		return report, e
	}
	if !acquired {
		return report, ErrBusy
	}
	courses, e := s.DB.Load(ctx)
	if e != nil {
		return report, e
	}
	var course *catalog.Course
	for i := range courses {
		if courses[i].ID == id {
			course = &courses[i]
			break
		}
	}
	if course == nil {
		return report, ErrAI
	}
	client := s.Client
	if client == nil {
		client = NewClient()
		defer client.CloseIdleConnections()
	}
	candidate, docs, e := aiFetchDocuments(ctx, s.DB, client, *course)
	if e != nil {
		return report, e
	}
	docs = s.aiDocuments(docs)
	now := time.Now().UTC()
	result, e := s.resolver().Resolve(ctx, id, candidate, docs, now)
	report.Model = result.ModelName
	report.Cached = result.Cached
	report.Tokens = result.Usage.Total
	report.DiscardedClaims = result.Extraction.DiscardedClaims
	if e != nil {
		return report, e
	}
	facts, e := aiCompile(candidate, docs, result.Extraction, result.Verification, now)
	if e != nil {
		return report, e
	}
	report.Recommendations = facts.Recommendations
	for _, i := range result.Verification.Verified {
		report.VerifiedClaims = append(report.VerifiedClaims, result.Extraction.Claims[i])
	}
	return report, nil
}
