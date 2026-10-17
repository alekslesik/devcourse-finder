package updater

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
)

type AIConfig struct {
	Key                 string `json:"-"`
	Model               string
	VerifyModel         string
	DeepSeekKey         string `json:"-"`
	DeepSeekModel       string
	DeepSeekVerifyModel string
	BatchSize           int
	DailyLimit          int
}

func LoadAIConfig() (AIConfig, error) {
	c := AIConfig{DeepSeekKey: os.Getenv("DEEPSEEK_API_KEY"), DeepSeekModel: os.Getenv("DEEPSEEK_MODEL"), DeepSeekVerifyModel: os.Getenv("DEEPSEEK_VERIFY_MODEL"), Key: os.Getenv("OPENAI_API_KEY"), Model: os.Getenv("OPENAI_MODEL"), VerifyModel: os.Getenv("OPENAI_VERIFY_MODEL"), BatchSize: 20, DailyLimit: 40}
	if c.Model == "" {
		c.Model = "gpt-4.1-mini"
	}
	if c.VerifyModel == "" {
		c.VerifyModel = "gpt-4.1"
	}
	if c.DeepSeekModel == "" {
		c.DeepSeekModel = "deepseek-flash"
	}
	if c.DeepSeekVerifyModel == "" {
		c.DeepSeekVerifyModel = "deepseek-v4-pro"
	}
	if strings.TrimSpace(c.Key) == "" && strings.TrimSpace(c.DeepSeekKey) == "" || !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`).MatchString(c.Model) || !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`).MatchString(c.VerifyModel) {
		return c, ErrAI
	}
	for _, model := range []string{c.DeepSeekModel, c.DeepSeekVerifyModel} {
		if !regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`).MatchString(model) {
			return c, ErrAI
		}
	}
	for _, item := range []struct {
		name   string
		target *int
		max    int
	}{{"CATALOG_AI_BATCH_SIZE", &c.BatchSize, 40}, {"CATALOG_AI_DAILY_REQUEST_LIMIT", &c.DailyLimit, 200}} {
		if raw := os.Getenv(item.name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 || n > item.max {
				return c, ErrAI
			}
			*item.target = n
		}
	}
	return c, nil
}

type AIService struct {
	DB     *store.DB
	Config AIConfig
	Model  AIModel
	Client *http.Client
}
type AIRunResult struct {
	ID        int64  `json:"run_id"`
	Attempted int    `json:"attempted"`
	Queued    int    `json:"queued"`
	Cached    int    `json:"cached"`
	Failed    int    `json:"failed"`
	Tokens    int    `json:"tokens"`
	Status    string `json:"status"`
}

func (s *AIService) fail(ctx context.Context, id, code string) error {
	_, err := s.DB.Pool.Exec(ctx, `UPDATE catalog_ai_sources SET code=$2,failures=failures+1,next_attempt_at=now()+least(24,2*greatest(1,failures+1))*interval '1 hour' WHERE course_id=$1`, id, code)
	return err
}
func (s *AIService) Run(ctx context.Context) (result AIRunResult, err error) {
	if s.Config.BatchSize < 1 || s.Config.BatchSize > 40 || s.Config.DailyLimit < 1 || s.Config.DailyLimit > 200 {
		return result, ErrAI
	}
	lock, err := s.DB.Pool.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		lock.Rollback(cleanup)
	}()
	var acquired bool
	if err = lock.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(829116)").Scan(&acquired); err != nil {
		return result, err
	}
	if !acquired {
		return result, ErrBusy
	}
	if err = s.DB.Pool.QueryRow(ctx, "INSERT INTO catalog_ai_runs DEFAULT VALUES RETURNING id").Scan(&result.ID); err != nil {
		return result, err
	}
	result.Status = "failed"
	defer func() {
		finish, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, e := s.DB.Pool.Exec(finish, "UPDATE catalog_ai_runs SET finished_at=now(),status=$2,attempted=$3,queued=$4,cached=$5,failed=$6,tokens=$7 WHERE id=$1", result.ID, result.Status, result.Attempted, result.Queued, result.Cached, result.Failed, result.Tokens)
		if err == nil {
			err = e
		}
	}()
	client := s.Client
	if client == nil {
		client = NewClient()
		defer client.CloseIdleConnections()
	}
	stepikAvailable, err := aiStepikAvailable(ctx, s.DB, client)
	if err != nil {
		return result, err
	}
	if !stepikAvailable {
		slog.Warn("AI source paused", "provider", "stepik", "code", "source_unavailable")
	}
	rows, err := s.DB.Pool.Query(ctx, `SELECT c.id FROM courses c LEFT JOIN catalog_ai_sources a ON a.course_id=c.id WHERE c.status='published' AND NOT c.demo AND c.source LIKE ANY($2) AND ($3 OR c.source NOT LIKE 'https://stepik.org/course/%') AND (a.next_attempt_at IS NULL OR a.next_attempt_at<=now()) ORDER BY a.attempted_at NULLS FIRST,c.id LIMIT $1`, s.Config.BatchSize, aiSourcePatterns(), stepikAvailable)
	if err != nil {
		return result, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return result, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	courses, err := s.DB.Load(ctx)
	if err != nil {
		return result, err
	}
	byID := map[string]catalog.Course{}
	for _, c := range courses {
		byID[c.ID] = c
	}
	model := s.Model
	if model == nil {
		model = &OpenAIModel{Key: s.Config.Key, Model: s.Config.Model, VerifyModel: s.Config.VerifyModel}
	}
	primary := &AIEngine{DB: s.DB, Model: model, ModelName: "openai:" + s.Config.Model + "+" + s.Config.VerifyModel, DailyLimit: s.Config.DailyLimit}
	resolver := aiFallbackResolver{primary: primary}
	if s.Model == nil && strings.TrimSpace(s.Config.Key) == "" {
		resolver.primary = nil
	}
	if strings.TrimSpace(s.Config.DeepSeekKey) != "" {
		resolver.fallback = &AIEngine{
			DB:         s.DB,
			Model:      &DeepSeekModel{Key: s.Config.DeepSeekKey, Model: s.Config.DeepSeekModel, VerifyModel: s.Config.DeepSeekVerifyModel},
			ModelName:  "deepseek:" + s.Config.DeepSeekModel + "+" + s.Config.DeepSeekVerifyModel,
			DailyLimit: s.Config.DailyLimit,
		}
	}
	publisher := Service{DB: s.DB}
	for _, id := range ids {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 10*time.Second {
			break
		}
		c, ok := byID[id]
		if !ok {
			continue
		}
		at := time.Now().UTC().Truncate(time.Microsecond)
		if _, err = s.DB.Pool.Exec(ctx, `INSERT INTO catalog_ai_sources(course_id,attempted_at,next_attempt_at,code) VALUES($1,$2,now()+interval '15 minutes','reading') ON CONFLICT(course_id) DO UPDATE SET attempted_at=$2,next_attempt_at=now()+interval '15 minutes',code='reading'`, id, at); err != nil {
			return result, err
		}
		result.Attempted++
		local, cancel := context.WithTimeout(ctx, 300*time.Second)
		candidate, docs, readErr := aiFetchDocuments(local, s.DB, client, c)
		code := "source_unavailable"
		if readErr != nil {
			if detail := rejectionCode(readErr, ""); detail != "" {
				code = "source_" + detail
			}
		}
		modelUsed, cached, tokens := "", false, 0
		discarded, verified := 0, 0
		if readErr == nil {
			var resolved AIResolved
			resolved, readErr = resolver.Resolve(local, id, candidate, docs, at)
			modelUsed, cached, tokens = resolved.ModelName, resolved.Cached, resolved.Usage.Total
			discarded = resolved.Extraction.DiscardedClaims
			if readErr == nil {
				verified = len(resolved.Verification.Verified)
			}
			code = "invalid_ai_response"
			result.Tokens += resolved.Usage.Total
			if readErr == nil {
				event := AIEvent{Version: 1, CourseID: id, IdentityDigest: aiIdentityDigest(c), Candidate: candidate, ObservedAt: at, Model: resolved.ModelName, Policy: aiEffectivePolicy(), Documents: docs, Extraction: resolved.Extraction, Verification: resolved.Verification}
				readErr = publisher.enqueueAI(local, event)
				code = "ai_queue_failed"
				if readErr == nil {
					result.Queued++
					if resolved.Cached {
						result.Cached++
					}
					code = "queued"
				}
			}
		}
		cancel()
		if readErr != nil {
			result.Failed++
			var providerError *AIProviderError
			if errors.As(readErr, &providerError) {
				code = providerError.Code
			}
			if errors.Is(readErr, ErrAIBudget) {
				code = "daily_budget_exhausted"
			}
			if err = s.fail(ctx, id, code); err != nil {
				return result, err
			}
		}
		slog.Info("AI course checked", "course_id", id, "code", code, "model", modelUsed, "cached", cached, "tokens", tokens, "discarded_claims", discarded, "verified_claims", verified)
		if aiProviderUnavailable(readErr) {
			break
		}
		if errors.Is(readErr, ErrAIBudget) {
			break
		}
	}
	result.Status = "completed"
	if result.Failed > 0 {
		result.Status = "partial"
	}
	return result, nil
}
func (s *AIService) Serve(ctx context.Context) error {
	if err := heartbeatWorker(ctx, s.DB, "ai"); err != nil {
		return err
	}
	beats, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		tick := time.NewTicker(30 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-beats.Done():
				return
			case <-tick.C:
				pulse, done := context.WithTimeout(beats, 5*time.Second)
				if heartbeatWorker(pulse, s.DB, "ai") != nil && beats.Err() == nil {
					slog.Error("AI heartbeat failed", "code", "heartbeat_failed")
				}
				done()
			}
		}
	}()
	for {
		var last *time.Time
		if err := s.DB.Pool.QueryRow(ctx, "SELECT max(started_at) FROM catalog_ai_runs").Scan(&last); err != nil {
			return err
		}
		if aiScheduledRunDue(time.Now(), last) {
			run, cancel := context.WithTimeout(ctx, 30*time.Minute)
			_, err := s.Run(run)
			cancel()
			if err != nil && !errors.Is(err, ErrBusy) && ctx.Err() == nil {
				slog.Error("AI collection failed", "code", "ai_collection_failed")
			}
		}
		_, next := aiScheduleSlots(time.Now())
		slog.Info("next AI collection scheduled", "at", next.Format(time.RFC3339), "timezone", "Europe/Moscow")
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}
func AIHealth(ctx context.Context, db *store.DB) error {
	var alive bool
	if err := db.Pool.QueryRow(ctx, "SELECT (value#>>'{}')::timestamptz>now()-interval '90 seconds' FROM settings WHERE key='updater_heartbeat_ai'").Scan(&alive); err != nil {
		return err
	}
	if !alive {
		return ErrAI
	}
	return nil
}
func AIStatus(ctx context.Context, db *store.DB, w io.Writer) error {
	var dayRequests int
	var dayTokens int64
	err := db.Pool.QueryRow(ctx, "SELECT requests,total_tokens FROM catalog_ai_daily_budget WHERE day=(now() AT TIME ZONE 'UTC')::date").Scan(&dayRequests, &dayTokens)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var pending int
	if err = db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_ai_observations WHERE processed_at IS NULL").Scan(&pending); err != nil {
		return err
	}
	coverage, err := aiCoverage(ctx, db)
	if err != nil {
		return err
	}
	health, err := aiProviderHealth(ctx, db)
	if err != nil {
		return err
	}
	if err = json.NewEncoder(w).Encode(map[string]any{"provider_health": health, "provider_coverage": coverage, "ai_collector": true, "requests_reserved_today": dayRequests, "tokens_today": dayTokens, "pending": pending, "policy": aiEffectivePolicy(), "supported_providers": aiProviderIDs()}); err != nil {
		return err
	}
	rows, err := db.Pool.Query(ctx, "SELECT course_id,code,attempted_at,verified_at,next_attempt_at,failures FROM catalog_ai_sources ORDER BY attempted_at DESC LIMIT 40")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, code string
		var at, next time.Time
		var verified *time.Time
		var failures int
		if err = rows.Scan(&id, &code, &at, &verified, &next, &failures); err != nil {
			return err
		}
		if err = json.NewEncoder(w).Encode(map[string]any{"course_id": id, "code": code, "attempted_at": at, "verified_at": verified, "next_attempt_at": next, "failures": failures}); err != nil {
			return err
		}
	}
	return rows.Err()
}
