package updater

import (
	"context"
	"sort"
	"time"
)

// Fill filter gaps across providers before refreshing complete profiles. Each
// provider gets a turn even while another has a large never-attempted backlog.
// Due times, provider pauses and manual holds remain eligibility constraints.
func (s AIService) selectAICourses(ctx context.Context, stepikAvailable bool) ([]string, error) {
	rows, err := s.DB.Pool.Query(ctx, `SELECT c.id,a.attempted_at,($4<>'' OR a.next_attempt_at IS NULL OR a.next_attempt_at<=now()) FROM courses c LEFT JOIN catalog_ai_sources a ON a.course_id=c.id WHERE c.status='published' AND NOT c.demo AND coalesce(a.code,'')<>'manual_review_required' AND c.source LIKE ANY($1) AND ($2 OR c.source NOT LIKE 'https://stepik.org/course/%') AND (coalesce(cardinality($3::text[]),0)=0 OR c.id=ANY($3::text[]))`, aiSourcePatterns(), stepikAvailable, s.Config.CourseIDs, s.TargetCourseID)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id, provider string
		attempted    *time.Time
		missing      int
		due          bool
	}
	queue := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.attempted, &c.due); err != nil {
			rows.Close()
			return nil, err
		}
		queue = append(queue, c)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	courses, err := s.DB.Load(ctx)
	if err != nil {
		return nil, err
	}
	missing := map[string]int{}
	providers := map[string]string{}
	for _, c := range courses {
		if source, ok := aiCandidateURL(c.Source); ok {
			missing[c.ID] = aiMissingCount(c)
			providers[c.ID] = source.Adapter
		}
	}
	lastAttempt := map[string]time.Time{}
	valid := queue[:0]
	for _, c := range queue {
		if n, ok := missing[c.id]; ok {
			c.missing, c.provider = n, providers[c.id]
			// Recent work can be cooling down. Retain its provider timestamp for
			// fairness without making that course eligible for another attempt.
			if c.attempted != nil && c.attempted.After(lastAttempt[c.provider]) {
				lastAttempt[c.provider] = *c.attempted
			}
			if c.due {
				valid = append(valid, c)
			}
		}
	}
	queue = valid
	sort.Slice(queue, func(i, j int) bool {
		a, b := queue[i], queue[j]
		if (a.missing > 0) != (b.missing > 0) {
			return a.missing > 0
		}
		if (a.attempted == nil) != (b.attempted == nil) {
			return a.attempted == nil
		}
		if a.missing != b.missing {
			return a.missing > b.missing
		}
		if a.attempted != nil && !a.attempted.Equal(*b.attempted) {
			return a.attempted.Before(*b.attempted)
		}
		return a.id < b.id
	})
	ids := []string{}
	for start := 0; start < len(queue) && len(ids) < s.Config.BatchSize; {
		end := start + 1
		for end < len(queue) && (queue[end].missing > 0) == (queue[start].missing > 0) {
			end++
		}
		pools := map[string][]candidate{}
		for _, c := range queue[start:end] {
			pools[c.provider] = append(pools[c.provider], c)
		}
		order := []string{}
		for provider := range pools {
			order = append(order, provider)
		}
		sort.Slice(order, func(i, j int) bool {
			a, b := lastAttempt[order[i]], lastAttempt[order[j]]
			if !a.Equal(b) {
				return a.Before(b)
			}
			return order[i] < order[j]
		})
		for round, remaining := 0, end-start; remaining > 0 && len(ids) < s.Config.BatchSize; round++ {
			for _, provider := range order {
				pool := pools[provider]
				if round < len(pool) && len(ids) < s.Config.BatchSize {
					ids = append(ids, pool[round].id)
					remaining--
				}
			}
		}
		start = end
	}
	return ids, nil
}
