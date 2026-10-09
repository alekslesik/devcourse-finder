package updater

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"devcourse-finder/catalog"
)

// The sum of all configured provider lanes fits below the global cap.
const batchLimit = 210
const stepikNewLimit = 120
const stepikRefreshLimit = 30
const providerLaneLimit = 6

type QueueWork struct {
	Candidate
	State    string
	Failures int
}
type BatchStats struct{ Attempted, Failed, Published, Seen int }
type publishCandidate func(context.Context, Candidate, catalog.Course, Observation, string) (string, int, error)

func (s *Service) discover(ctx context.Context, client *http.Client) (BatchStats, error) {
	stats := BatchStats{}
	for _, feed := range s.Config.Discovery {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		stats.Attempted++
		count, err := s.discoverFeed(ctx, client, feed)
		code := "verified"
		fail := 0
		if err != nil {
			code = "feed_unavailable"
			fail = 1
			stats.Failed++
		} else {
			stats.Seen += count
		}
		var failures int
		dbErr := s.DB.Pool.QueryRow(ctx, `INSERT INTO discovery_feeds(feed_id,attempted_at,verified_at,code,failures,candidates)
   VALUES($1,now(),CASE WHEN $3=0 THEN now() END,$2,$3,$4)
   ON CONFLICT(feed_id) DO UPDATE SET attempted_at=now(),verified_at=CASE WHEN $3=0 THEN now() ELSE discovery_feeds.verified_at END,
   code=$2,failures=CASE WHEN $3=0 THEN 0 ELSE discovery_feeds.failures+1 END,candidates=$4 RETURNING failures`, feed.ID, code, fail, count).Scan(&failures)
		if dbErr != nil {
			return stats, dbErr
		}
		level := slog.LevelInfo
		if failures > 0 {
			level = slog.LevelWarn
		}
		if failures >= 3 {
			level = slog.LevelError
		}
		slog.Log(ctx, level, "discovery feed checked", "feed_id", feed.ID, "code", code, "consecutive_failures", failures, "candidates_seen", count)
	}
	return stats, nil
}
func (s *Service) queueWork(ctx context.Context) ([]QueueWork, error) {
	adapters := []string{}
	for _, f := range s.Config.Discovery {
		adapters = append(adapters, f.Adapter)
	}
	if len(adapters) == 0 {
		return nil, nil
	}
	rows, err := s.DB.Pool.Query(ctx, `WITH lanes AS (
  SELECT adapter,external_id,canonical_url,feed_id,state,failures,
   row_number() OVER(PARTITION BY adapter,(state='published') ORDER BY attempted_at NULLS FIRST,first_seen_at,external_id) AS rank
  FROM catalog_candidates WHERE next_attempt_at<=now() AND adapter=ANY($1::text[])
 ) SELECT adapter,external_id,canonical_url,feed_id,state,failures FROM lanes
 WHERE rank<=CASE WHEN adapter='stepik' THEN CASE WHEN state='published' THEN $3::bigint ELSE $4::bigint END ELSE $2::bigint END
 ORDER BY rank,adapter,(state='published') DESC LIMIT $5`, adapters, providerLaneLimit, stepikRefreshLimit, stepikNewLimit, batchLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	work := []QueueWork{}
	for rows.Next() {
		var w QueueWork
		if err = rows.Scan(&w.Adapter, &w.ExternalID, &w.URL, &w.FeedID, &w.State, &w.Failures); err != nil {
			return nil, err
		}
		work = append(work, w)
	}
	return work, rows.Err()
}
func retryDelay(failures int) time.Duration {
	if failures < 1 {
		return 12 * time.Hour
	}
	if failures > 3 {
		failures = 3
	}
	delay := time.Duration(1<<failures) * 12 * time.Hour
	if delay > 72*time.Hour {
		return 72 * time.Hour
	}
	return delay
}
func (s *Service) processBatch(ctx context.Context, client *http.Client, publish publishCandidate) (BatchStats, error) {
	stats := BatchStats{}
	var basicsPolicy []byte
	var basicsPolicyDigest string
	policyAttempted := false
	work, err := s.queueWork(ctx)
	if err != nil {
		return stats, err
	}
	for _, w := range work {
		if ctx.Err() != nil {
			return stats, ctx.Err()
		}
		// Do not claim work that cannot reasonably finish within this run's budget.
		reserve := 35 * time.Second
		if w.Adapter == "codebasics" && !policyAttempted {
			reserve = 70 * time.Second // One detail plus one pricing-policy fetch.
		}
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < reserve {
			break
		}
		if _, err = s.DB.Pool.Exec(ctx, `UPDATE catalog_candidates SET attempted_at=now(),next_attempt_at=now()+interval '15 minutes' WHERE adapter=$1 AND external_id=$2`, w.Adapter, w.ExternalID); err != nil {
			return stats, err
		}
		stats.Attempted++
		body, digest, err := fetch(ctx, client, candidateEndpoint(w.Candidate))
		code := "source_unavailable"
		state := "rejected"
		failures := w.Failures + 1
		delay := retryDelay(failures)
		if err == nil {
			var record catalog.Course
			var o Observation
			var parseErr error
			if w.Adapter == "codebasics" {
				if !policyAttempted {
					policyAttempted = true
					basicsPolicy, basicsPolicyDigest, parseErr = fetch(ctx, client, "https://code-basics.com/ru")
				}
				if parseErr == nil {
					record, o, parseErr = collectCodeBasics(body, basicsPolicy, w.Candidate, time.Now().UTC())
				}
				digest = fingerprint([]byte(digest + ":" + basicsPolicyDigest))
			} else {
				record, o, parseErr = collectCandidate(body, w.Candidate, time.Now().UTC())
			}
			code = "invalid_course"
			if parseErr == nil {
				var published int
				code, published, err = publish(ctx, w.Candidate, record, o, digest)
				if err != nil {
					return stats, err
				}
				stats.Published += published
				failures = 0
				delay = 12 * time.Hour
				switch code {
				case "verified":
					state = "published"
				case "pending_confirmation":
					state = w.State
				case "protected_course", "protected_offer", "protected_identity":
					state = "protected"
					delay = 7 * 24 * time.Hour
				default:
					state = "rejected"
					delay = 72 * time.Hour
				}
			}
		}
		if failures > 0 {
			stats.Failed++
			// A failed detail read breaks consecutive anomaly confirmation just
			// as it does for curated sources; retries cannot skip missing evidence.
			if _, err = s.DB.Pool.Exec(ctx, `DELETE FROM updater_candidates WHERE source_id IN
			 (SELECT course_id FROM catalog_identities WHERE adapter=$1 AND external_id=$2)`, w.Adapter, w.ExternalID); err != nil {
				return stats, err
			}
		}
		// Each completed item persists its own retry/refresh time. A process killed
		// after claiming leaves only a 15-minute lease, never a permanent in-flight row.
		if _, err = s.DB.Pool.Exec(ctx, `UPDATE catalog_candidates SET state=$3,code=$4,failures=$5,next_attempt_at=now()+$6*interval '1 second',evidence_sha256=NULLIF($7,'') WHERE adapter=$1 AND external_id=$2`, w.Adapter, w.ExternalID, state, code, failures, delay.Seconds(), digest); err != nil {
			return stats, err
		}
		level := slog.LevelInfo
		if failures > 0 {
			level = slog.LevelWarn
		}
		if failures >= 3 {
			level = slog.LevelError
		}
		slog.Log(ctx, level, "catalog candidate checked", "adapter", w.Adapter, "external_id", w.ExternalID, "code", code, "state", state, "consecutive_failures", failures)
	}
	return stats, nil
}
