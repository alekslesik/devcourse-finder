package updater

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

type aiResolutionEngine interface {
	Resolve(context.Context, string, Candidate, []AIDocument, time.Time) (AIResolved, error)
}

// One resolver lives for a single collection run. Once OpenAI is unavailable,
// use DeepSeek for the rest of that run; the next run tries OpenAI again.
type aiFallbackResolver struct {
	primary  aiResolutionEngine
	fallback aiResolutionEngine
	usingFallback bool
}

func aiProviderUnavailable(err error) bool {
	var provider *AIProviderError
	if !errors.As(err, &provider) {
		return false
	}
	switch provider.Code {
	case "openai_unavailable", "openai_authentication_failed", "openai_insufficient_quota",
		"openai_rate_limited", "openai_model_unavailable", "openai_protocol_rejected",
		"deepseek_unavailable", "deepseek_authentication_failed", "deepseek_insufficient_quota",
		"deepseek_rate_limited", "deepseek_model_unavailable", "deepseek_protocol_rejected":
		return true
	}
	return false
}

func (r *aiFallbackResolver) Resolve(ctx context.Context, id string, c Candidate, docs []AIDocument, at time.Time) (AIResolved, error) {
	if err := ctx.Err(); err != nil {
		return AIResolved{}, err
	}
	if r.usingFallback {
		return r.fallback.Resolve(ctx, id, c, docs, at)
	}
	var primary AIResolved
	var err error
	if r.primary != nil {
		primary, err = r.primary.Resolve(ctx, id, c, docs, at)
		if err == nil || !aiProviderUnavailable(err) || r.fallback == nil || ctx.Err() != nil {
			return primary, err
		}
	} else if r.fallback == nil {
		return primary, ErrAI
	}
	reason := "openai_not_configured"
	var failure *AIProviderError
	if errors.As(err, &failure) {
		reason = failure.Code
	}
	slog.Warn("AI provider fallback", "from", "openai", "to", "deepseek", "code", reason)
	r.usingFallback = true
	// Restart the complete analysis. Never combine a failed OpenAI extraction
	// with DeepSeek verification or relabel an OpenAI cache entry as DeepSeek.
	backup, fallbackErr := r.fallback.Resolve(ctx, id, c, docs, at)
	backup.Usage.Prompt += primary.Usage.Prompt
	backup.Usage.Completion += primary.Usage.Completion
	backup.Usage.Total += primary.Usage.Total
	return backup, fallbackErr
}
