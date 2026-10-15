package updater

import (
	"context"
	"testing"
	"time"
)

type failingProviderModel struct {
	fakeAIModel
	verifyFailure bool
}

func (m *failingProviderModel) Extract(ctx context.Context, c Candidate, docs []AIDocument) (AIExtraction, AIUsage, error) {
	x, usage, err := m.fakeAIModel.Extract(ctx, c, docs)
	if !m.verifyFailure { err = &AIProviderError{Code: "openai_insufficient_quota"} }
	return x, usage, err
}

func (m *failingProviderModel) Verify(ctx context.Context, c Candidate, docs []AIDocument, x AIExtraction) (AIVerification, AIUsage, error) {
	v, usage, _ := m.fakeAIModel.Verify(ctx, c, docs, x)
	return v, usage, &AIProviderError{Code: "openai_unavailable"}
}

func TestAIFallbackCompleteAnalysisSeparateCacheAndSharedBudget(t *testing.T) {
	for _, verifyFailure := range []bool{false, true} {
		t.Run(map[bool]string{false:"extraction",true:"verification"}[verifyFailure], func(t *testing.T) {
			record, primary, docs := aiImportedCourse(t)
			primary.DailyLimit = 4
			primary.ModelName = "openai:mini+review"
			failed := &failingProviderModel{verifyFailure: verifyFailure}
			primary.Model = failed
			backupModel := &fakeAIModel{}
			backup := &AIEngine{DB: primary.DB, Model: backupModel, ModelName:"deepseek:flash+pro", DailyLimit:4}
			r := aiFallbackResolver{primary:primary, fallback:backup}
			c, _, _, _ := aiFixture()
			out, err := r.Resolve(context.Background(), record.ID, c, docs, time.Now())
			wantTokens := 250
			if verifyFailure { wantTokens = 300 }
			if err != nil || out.ModelName != backup.ModelName || out.Cached || out.Usage.Total != wantTokens || backupModel.calls != 2 { t.Fatal(out, err, backupModel.calls) }
			var model string
			if err = primary.DB.Pool.QueryRow(context.Background(), "SELECT model FROM catalog_ai_cache").Scan(&model); err != nil || model != backup.ModelName { t.Fatal(model, err) }
			var requests, tokens int
			if err = primary.DB.Pool.QueryRow(context.Background(), "SELECT requests,total_tokens FROM catalog_ai_daily_budget").Scan(&requests,&tokens); err != nil || requests != 4 || tokens != wantTokens { t.Fatal(requests,tokens,err) }
			out, err = r.Resolve(context.Background(), record.ID, c, docs, time.Now())
			if err != nil || !out.Cached || out.Usage.Total != 0 || backupModel.calls != 2 { t.Fatal(out,err) }
			if verifyFailure && failed.calls != 2 || !verifyFailure && failed.calls != 1 { t.Fatal("primary probed again within failed run",failed.calls) }
		})
	}
}

func TestAIFallbackCannotOverspendGlobalLimit(t *testing.T) {
	record, primary, docs := aiImportedCourse(t)
	primary.Model = &failingProviderModel{}
	backupModel := &fakeAIModel{}
	backup := &AIEngine{DB:primary.DB,Model:backupModel,ModelName:"deepseek:flash+pro",DailyLimit:2}
	r := aiFallbackResolver{primary:primary,fallback:backup}
	c, _, _, _ := aiFixture()
	if _, err := r.Resolve(context.Background(),record.ID,c,docs,time.Now()); err != ErrAIBudget || backupModel.calls != 0 { t.Fatal(err,backupModel.calls) }
}
