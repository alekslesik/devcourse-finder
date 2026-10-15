package updater

import (
	"context"
	"errors"
	"testing"
	"time"
)

type resolutionStub struct {
	calls int
	result AIResolved
	err error
}

func (s *resolutionStub) Resolve(context.Context, string, Candidate, []AIDocument, time.Time) (AIResolved, error) {
	s.calls++
	return s.result, s.err
}

func TestAIFallbackOnlyForProviderFailureAndSticksForRun(t *testing.T) {
	c, docs, _, _ := aiFixture()
	for _, code := range []string{"openai_unavailable", "openai_insufficient_quota", "openai_authentication_failed", "openai_rate_limited", "openai_model_unavailable", "openai_protocol_rejected"} {
		t.Run(code, func(t *testing.T) {
			primary := &resolutionStub{err: &AIProviderError{Code: code}, result: AIResolved{Usage: AIUsage{Prompt: 10, Total: 10}}}
			backup := &resolutionStub{result: AIResolved{ModelName: "deepseek:flash+pro", Usage: AIUsage{Prompt: 20, Total: 20}}}
			r := aiFallbackResolver{primary: primary, fallback: backup}
			out, err := r.Resolve(context.Background(), "course", c, docs, time.Now())
			if err != nil || out.ModelName != "deepseek:flash+pro" || out.Usage.Total != 30 { t.Fatal(out, err) }
			out, err = r.Resolve(context.Background(), "course", c, docs, time.Now())
			if err != nil || out.Usage.Total != 20 || primary.calls != 1 || backup.calls != 2 { t.Fatal(out, err, primary.calls, backup.calls) }
		})
	}
}

func TestAIFallbackDoesNotBypassValidationBudgetOrCancellation(t *testing.T) {
	c, docs, _, _ := aiFixture()
	for _, failure := range []error{ErrAI, ErrAIBudget, context.Canceled, context.DeadlineExceeded, &AIProviderError{Code: "openai_request_rejected"}} {
		primary := &resolutionStub{err: failure}
		backup := &resolutionStub{}
		r := aiFallbackResolver{primary: primary, fallback: backup}
		if _, err := r.Resolve(context.Background(), "course", c, docs, time.Now()); !errors.Is(err, failure) || backup.calls != 0 { t.Fatal(err, backup.calls) }
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	primary, backup := &resolutionStub{}, &resolutionStub{}
	r := aiFallbackResolver{primary: primary, fallback: backup}
	if _, err := r.Resolve(ctx, "course", c, docs, time.Now()); !errors.Is(err, context.Canceled) || primary.calls+backup.calls != 0 { t.Fatal(err) }
}

func TestAIPrimarySuccessAndAbsentBackup(t *testing.T) {
	c, docs, _, _ := aiFixture()
	primary, backup := &resolutionStub{result: AIResolved{ModelName: "openai:mini+review"}}, &resolutionStub{}
	r := aiFallbackResolver{primary: primary, fallback: backup}
	out, err := r.Resolve(context.Background(), "course", c, docs, time.Now())
	if err != nil || out.ModelName != "openai:mini+review" || backup.calls != 0 { t.Fatal(out, err) }
	primary.err = &AIProviderError{Code: "openai_insufficient_quota"}
	r.fallback = nil
	if _, err = r.Resolve(context.Background(), "course", c, docs, time.Now()); !aiProviderUnavailable(err) { t.Fatal(err) }
	r = aiFallbackResolver{fallback: backup}
	if _, err = r.Resolve(context.Background(), "course", c, docs, time.Now()); err != nil || backup.calls != 1 { t.Fatal(err) }
}

func TestAIBackupProtocolErrorStopsCollection(t *testing.T) {
	c, docs, _, _ := aiFixture()
	primary := &resolutionStub{err: &AIProviderError{Code: "openai_unavailable"}}
	backup := &resolutionStub{err: &AIProviderError{Code: "deepseek_protocol_rejected"}}
	r := aiFallbackResolver{primary: primary, fallback: backup}
	_, err := r.Resolve(context.Background(), "course", c, docs, time.Now())
	if err == nil || err.Error() != "deepseek_protocol_rejected" || !aiProviderUnavailable(err) || primary.calls != 1 || backup.calls != 1 {
		t.Fatal("deterministic backup error did not stop the run", err)
	}
}
