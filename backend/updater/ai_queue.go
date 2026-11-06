package updater

import (
	"context"
	"sort"
	"time"
)

// Eligibility, due times and provider pauses are applied before ranking. Fresh
// courses come first; within each group, fill filter gaps before refreshing
// complete profiles. The oldest attempt breaks ties so retries cannot monopolize
// a batch. Explicit verification retains its existing due-time exemption.
func (s AIService) selectAICourses(ctx context.Context, stepikAvailable bool) ([]string, error) {
	rows, err := s.DB.Pool.Query(ctx, `SELECT c.id,a.attempted_at FROM courses c LEFT JOIN catalog_ai_sources a ON a.course_id=c.id WHERE c.status='published' AND NOT c.demo AND c.source LIKE ANY($1) AND ($2 OR c.source NOT LIKE 'https://stepik.org/course/%') AND (coalesce(cardinality($3::text[]),0)=0 OR c.id=ANY($3::text[])) AND ($4<>'' OR a.next_attempt_at IS NULL OR a.next_attempt_at<=now())`, aiSourcePatterns(), stepikAvailable, s.Config.CourseIDs, s.TargetCourseID)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id        string
		attempted *time.Time
		missing   int
	}
	queue := []candidate{}
	for rows.Next() {
		var c candidate
		if err = rows.Scan(&c.id, &c.attempted); err != nil {
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
	for _, c := range courses {
		if _, ok := aiCandidateURL(c.Source); ok {
			missing[c.ID] = aiMissingCount(c)
		}
	}
	valid := queue[:0]
	for _, c := range queue {
		if n, ok := missing[c.id]; ok {
			c.missing = n
			valid = append(valid, c)
		}
	}
	queue = valid
	sort.Slice(queue, func(i, j int) bool {
		a, b := queue[i], queue[j]
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
	if len(queue) > s.Config.BatchSize {
		queue = queue[:s.Config.BatchSize]
	}
	ids := []string{}
	for _, c := range queue {
		ids = append(ids, c.id)
	}
	return ids, nil
}
