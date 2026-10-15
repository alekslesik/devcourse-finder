package updater

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
)

var ErrAIBudget = errors.New("AI daily request budget exhausted")

type AIEngine struct {
	DB         *store.DB
	Model      AIModel
	ModelName  string
	DailyLimit int
}
type AIResolved struct {
	Extraction   AIExtraction
	Verification AIVerification
	Cached       bool
	Usage        AIUsage
	ModelName    string
}

// Reserve a complete extraction/verification pair atomically. Failed analyses
// consume the reserved pair conservatively, including an unused verification slot.
func (e *AIEngine) reserve(ctx context.Context) error {
	if e.DailyLimit < 2 {
		return ErrAIBudget
	}
	if e.DailyLimit > 200 {
		return ErrAI
	}
	var requests int
	err := e.DB.Pool.QueryRow(ctx, `INSERT INTO catalog_ai_daily_budget(day,requests,total_tokens) VALUES((now() AT TIME ZONE 'UTC')::date,2,0)
 ON CONFLICT(day) DO UPDATE SET requests=catalog_ai_daily_budget.requests+2 WHERE catalog_ai_daily_budget.requests<=$1-2 RETURNING requests`, e.DailyLimit).Scan(&requests)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrAIBudget
	}
	return err
}
func (e *AIEngine) recordUsage(ctx context.Context, u AIUsage) error {
	if !u.valid() {
		return ErrAI
	}
	_, err := e.DB.Pool.Exec(ctx, `UPDATE catalog_ai_daily_budget SET total_tokens=total_tokens+$1 WHERE day=(now() AT TIME ZONE 'UTC')::date`, u.Total)
	return err
}
func (e *AIEngine) Resolve(ctx context.Context, courseID string, c Candidate, docs []AIDocument, at time.Time) (AIResolved, error) {
	out := AIResolved{ModelName: e.ModelName}
	if !aiDocumentsValid(c, docs) {
		return out, ErrAI
	}
	digest, policy := aiBundleDigest(docs), aiEffectivePolicy()
	var x, v []byte
	err := e.DB.Pool.QueryRow(ctx, "SELECT extraction,verification FROM catalog_ai_cache WHERE course_id=$1 AND bundle_digest=$2 AND model=$3 AND policy=$4", courseID, digest, e.ModelName, policy).Scan(&x, &v)
	if err == nil {
		if aiStrictJSON(x, &out.Extraction) != nil || aiStrictJSON(v, &out.Verification) != nil {
			return out, ErrAI
		}
		if _, err = aiCompile(c, docs, out.Extraction, out.Verification, at); err != nil {
			return out, err
		}
		out.Cached = true
		_, err = e.DB.Pool.Exec(ctx, "UPDATE catalog_ai_cache SET used_at=now() WHERE course_id=$1 AND bundle_digest=$2 AND model=$3 AND policy=$4", courseID, digest, e.ModelName, policy)
		return out, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	if e.Model == nil {
		return out, ErrAI
	}
	if err = e.reserve(ctx); err != nil {
		return out, err
	}
	out.Extraction, out.Usage, err = e.Model.Extract(ctx, c, docs)
	if usageErr := e.recordUsage(ctx, out.Usage); usageErr != nil {
		return out, usageErr
	}
	if err != nil {
		return out, err
	}
	out.Extraction, err = aiIsolateClaims(c, docs, out.Extraction, at)
	if err != nil {
		return out, err
	}
	out.Extraction, err = e.carryForward(ctx, courseID, c, docs, out.Extraction, at)
	if err != nil {
		return out, err
	}
	// Check factual shape, exact quotes and scopes before spending a verification request.
	if _, err = aiCompile(c, docs, out.Extraction, AIVerification{Complete: true}, at); err != nil {
		return out, err
	}
	var usage AIUsage
	out.Verification, usage, err = e.Model.Verify(ctx, c, docs, out.Extraction)
	if usageErr := e.recordUsage(ctx, usage); usageErr != nil {
		return out, usageErr
	}
	out.Usage.Prompt += usage.Prompt
	out.Usage.Completion += usage.Completion
	out.Usage.Total += usage.Total
	if err != nil {
		return out, err
	}
	if _, err = aiCompile(c, docs, out.Extraction, out.Verification, at); err != nil {
		return out, err
	}
	x, _ = json.Marshal(out.Extraction)
	v, _ = json.Marshal(out.Verification)
	_, err = e.DB.Pool.Exec(ctx, `INSERT INTO catalog_ai_cache(course_id,bundle_digest,model,policy,extraction,verification) VALUES($1,$2,$3,$4,$5::jsonb,$6::jsonb) ON CONFLICT DO NOTHING`, courseID, digest, e.ModelName, policy, x, v)
	if err != nil {
		return out, err
	}
	// At most three analyses per course, including old model/policy versions.
	_, err = e.DB.Pool.Exec(ctx, `DELETE FROM catalog_ai_cache WHERE (course_id,bundle_digest,model,policy) IN (SELECT course_id,bundle_digest,model,policy FROM (SELECT course_id,bundle_digest,model,policy,row_number() OVER(PARTITION BY course_id ORDER BY used_at DESC) rank FROM catalog_ai_cache WHERE course_id=$1) old WHERE rank>3)`, courseID)
	return out, err
}

// A changed document can retain a previously verified fact even if extraction
// overlooks it. It is carried forward ONLY as a proposal to the independent
// verifier, with its exact quote still present in the current scoped document.
func (e *AIEngine) carryForward(ctx context.Context, courseID string, c Candidate, docs []AIDocument, x AIExtraction, at time.Time) (AIExtraction, error) {
	rows, err := e.DB.Pool.Query(ctx, "SELECT extraction,verification FROM catalog_ai_cache WHERE course_id=$1 ORDER BY used_at DESC LIMIT 3", courseID)
	if err != nil {
		return x, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for _, claim := range x.Claims {
		seen[claim.OfferID+":"+claim.Field] = true
	}
	for rows.Next() {
		var rawX, rawV []byte
		if err = rows.Scan(&rawX, &rawV); err != nil {
			return x, err
		}
		var prior AIExtraction
		var verification AIVerification
		if aiStrictJSON(rawX, &prior) != nil || aiStrictJSON(rawV, &verification) != nil || prior.CourseURL != c.URL {
			continue
		}
		for _, i := range verification.Verified {
			if i < 0 || i >= len(prior.Claims) {
				continue
			}
			claim := prior.Claims[i]
			claim.QuoteID = "" // Revalidate the literal old quote even if segment boundaries changed.
			key := claim.OfferID + ":" + claim.Field
			if seen[key] || len(x.Claims) >= aiMaxClaims {
				continue
			}
			single := AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{claim}}
			if _, err = aiCompile(c, docs, single, AIVerification{Complete: true, Verified: []int{0}}, at); err != nil {
				continue
			}
			x.Claims = append(x.Claims, claim)
			seen[key] = true
		}
	}
	return x, rows.Err()
}
