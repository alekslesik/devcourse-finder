package updater

import (
	"context"
	"testing"
	"time"
)

func TestManualReviewHoldBlocksSelectionAndAlreadyQueuedPublication(t *testing.T) {
	record, engine, _ := aiImportedCourse(t)
	ctx := context.Background()
	_, err := engine.DB.Pool.Exec(ctx, "INSERT INTO catalog_ai_sources(course_id,attempted_at,next_attempt_at,code) VALUES($1,now()-interval '1 day',now()-interval '1 day','manual_review_required')", record.ID)
	if err != nil {
		t.Fatal(err)
	}
	service := AIService{DB: engine.DB, Config: AIConfig{BatchSize: 100, DailyLimit: 200}}
	for _, target := range []string{"", record.ID} {
		service.TargetCourseID = target
		ids, err := service.selectAICourses(ctx, true)
		if err != nil || len(ids) != 0 {
			t.Fatal("held course selected", ids, err)
		}
	}
	if err = service.fail(ctx, record.ID, "invalid_ai_response"); err != nil {
		t.Fatal(err)
	}
	c, docs, x, v := recommendationExtraction()
	docs = aiRecommendationDocuments(docs)
	event := AIEvent{RecommendationsOnly: true, Version: 1, CourseID: record.ID, IdentityDigest: aiIdentityDigest(record), Candidate: c, ObservedAt: time.Now().UTC().Truncate(time.Microsecond), Model: "fixture:recommendations", Policy: aiEffectivePolicy(), Documents: docs, Extraction: x, Verification: v}
	publisher := Service{DB: engine.DB}
	if err = publisher.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	var code string
	if err = engine.DB.Pool.QueryRow(ctx, "SELECT code FROM catalog_ai_sources WHERE course_id=$1", record.ID).Scan(&code); err != nil || code != "manual_review_required" {
		t.Fatal("hold overwritten", code, err)
	}
	if n, err := publisher.publishAI(ctx); err != nil || n != 0 {
		t.Fatal("held event published", n, err)
	}
	loaded, err := engine.DB.Load(ctx)
	if err != nil || loaded[0].Recommendations != nil {
		t.Fatal("held recommendation reached public catalog", err)
	}
	// Explicit release restores eligibility; preview itself remains read-only.
	_, err = engine.DB.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET code='pending',next_attempt_at=now() WHERE course_id=$1", record.ID)
	if err != nil {
		t.Fatal(err)
	}
	ids, err := service.selectAICourses(ctx, true)
	if err != nil || len(ids) != 1 || ids[0] != record.ID {
		t.Fatal(ids, err)
	}
}
