package updater

import (
	"bytes"
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestPurpleLegacyOwnershipUsesLastRelevantWriter(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	known := false
	price := int64(100000)
	c := catalog.Course{ID: "old-purple", Slug: "old-purple", Title: "Python", Summary: "Курс Python", Provider: "PurpleSchool", Language: "python", Direction: "unknown", Audience: []string{"unknown"}, Goals: []string{"unknown"}, Topics: []string{}, Source: "https://purpleschool.ru/course/python-start", Status: "published", CheckedAt: time.Now().UTC(), Offers: []catalog.Offer{{ID: "old-purple-course", Name: "Team project", URL: "https://purpleschool.ru/course/python-start", Price: &price, PriceKind: "exact", PriceCheckedAt: time.Now().UTC(), Schedule: "flexible", Enrollment: "continuous", SupportKnown: &known}}}
	domains := []string{"purpleschool.ru"}
	if err := db.Import(ctx, catalog.Dataset{Domains: domains, Courses: []catalog.Course{c}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	_, err := db.UpdateVerified(ctx, domains, "automatic-catalog-publisher", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
		old = append([]catalog.Course(nil), old...)
		old[0].Offers = append([]catalog.Offer(nil), old[0].Offers...)
		v := true
		old[0].Offers[0].SupportKnown = &v
		old[0].Offers[0].Review = true
		old[0].Offers[0].Mentor = true
		return old, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	check := func(want int) {
		t.Helper()
		tx, err := db.Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		old, err := db.Load(ctx)
		if err != nil {
			t.Fatal(err)
		}
		restored, err := restorePurpleLearningOwnership(ctx, tx, old)
		if err != nil || restored[0].Offers[0].LearningOwnership != want {
			t.Fatal(restored, err)
		}
	}
	check(learningReview | learningMentor)
	loaded, err := db.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Import(ctx, catalog.Dataset{Domains: domains, Courses: loaded}, "operator-confirmation", false); err != nil {
		t.Fatal(err)
	}
	// An unchanged automatic publication must not reclaim explicitly curated values.
	_, err = db.UpdateVerified(ctx, domains, "automatic-catalog-publisher", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) { return old, nil })
	if err != nil {
		t.Fatal(err)
	}
	check(0)
}

func TestLearningCoverageStatusHandlesNullAndReportsEvidence(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	known := false
	c := catalog.Course{ID: "status-python", Slug: "status-python", Title: "Python", Summary: "Курс Python", Provider: "Test", Language: "python", Direction: "unknown", Audience: []string{"unknown"}, Goals: []string{"unknown"}, Topics: []string{}, Source: "https://otus.ru/lessons/python", Status: "published", CheckedAt: time.Now().UTC(), Offers: []catalog.Offer{{ID: "null-facts", Name: "Unknown", URL: "https://otus.ru/lessons/python", PriceKind: "unknown", Schedule: "unknown", Enrollment: "open", SupportKnown: &known}, {ID: "known-facts", Name: "Verified", URL: "https://otus.ru/lessons/python", PriceKind: "unknown", Schedule: "unknown", Enrollment: "open", SupportKnown: &known, LearningConditions: learningFixture(Candidate{URL: "https://otus.ru/lessons/python"}, time.Now().UTC())}}}
	if err := db.Import(ctx, catalog.Dataset{Domains: []string{"otus.ru"}, Courses: []catalog.Course{c}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := learningStatus(ctx, db, &out); err != nil {
		t.Fatal(err)
	}
	var row struct {
		Offers, Duration, Review int
		Workload                 int        `json:"weekly_workload"`
		Oldest                   *time.Time `json:"oldest_fact_at"`
	}
	if err := json.Unmarshal(out.Bytes(), &row); err != nil || row.Offers != 2 || row.Duration != 1 || row.Workload != 1 || row.Review != 1 || row.Oldest == nil {
		t.Fatal(out.String(), err)
	}
}
