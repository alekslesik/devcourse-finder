package updater

import (
	"context"
	"testing"
	"time"
)

type diagnosticModel struct{ kind string }

func (m diagnosticModel) Extract(ctx context.Context, c Candidate, d []AIDocument) (AIExtraction, AIUsage, error) {
	_, _, x, _ := aiFixture()
	switch m.kind {
	case "extraction":
		return x, AIUsage{}, ErrAI
	case "grounding":
		x.Claims[0].Quote = "not on page"
		x.Claims[1].Quote = "not on page"
	case "empty":
		x.Claims = nil
	}
	return x, AIUsage{}, nil
}
func (m diagnosticModel) Verify(ctx context.Context, c Candidate, d []AIDocument, x AIExtraction) (AIVerification, AIUsage, error) {
	if m.kind == "validation" {
		return AIVerification{Complete: true, Verified: []int{999}}, AIUsage{}, nil
	}
	return AIVerification{Complete: true}, AIUsage{}, nil
}
func TestAIRejectionStagesAreFixedAndEmptyIsSuccessful(t *testing.T) {
	for _, kind := range []string{"extraction", "grounding", "validation", "empty"} {
		t.Run(kind, func(t *testing.T) {
			record, engine, docs := aiImportedCourse(t)
			engine.Model = diagnosticModel{kind}
			c, _, _, _ := aiFixture()
			got, err := engine.Resolve(context.Background(), record.ID, c, docs, time.Now())
			if kind == "empty" {
				if err != nil || got.Stage != "complete" || len(got.Verification.Verified) != 0 {
					t.Fatal(got, err)
				}
				return
			}
			if err != ErrAI || got.Stage != kind || got.Reason != "response_rejected" {
				t.Fatal(got, err)
			}
		})
	}
}
func TestAIWorkerReportsEmptyObservationWithoutClaimingUsefulEnrichment(t *testing.T) {
	_, engine, _ := aiImportedCourse(t)
	ctx := context.Background()
	service := AIService{DB: engine.DB, Config: AIConfig{Model: "fixture", BatchSize: 1, DailyLimit: 2}, Model: diagnosticModel{"empty"}, Client: aiSourceClient(200)}
	got, err := service.Run(ctx)
	if err != nil || got.Queued != 1 || got.Empty != 1 || got.Useful != 0 || got.VerifiedClaims != 0 {
		t.Fatal(got, err)
	}
	var empty, useful int
	if err = engine.DB.Pool.QueryRow(ctx, "SELECT empty_queued,useful_queued FROM catalog_ai_runs WHERE id=$1", got.ID).Scan(&empty, &useful); err != nil || empty != 1 || useful != 0 {
		t.Fatal(empty, useful, err)
	}
	pub := Service{DB: engine.DB, Config: productionConfig(t), Worker: "publisher"}
	if _, err = pub.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	codes, err := aiSourceCodes(ctx, engine.DB)
	if err != nil || codes["ai_no_change"] != 1 {
		t.Fatal(codes, err)
	}
}
