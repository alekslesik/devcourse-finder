package updater

import (
	"context"
	"devcourse-finder/catalog"
	"github.com/jackc/pgx/v5"
	"testing"
	"time"
)

func TestIdentityReusesCuratedIDsAndRejectsDuplicateTariffs(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	ctx := context.Background()
	existing := config.Template("stepik-golang")
	if err := db.Import(ctx, catalog.Dataset{Domains: config.Templates.Domains, Courses: []catalog.Course{existing}}, "test", false); err != nil {
		t.Fatal(err)
	}
	candidate := Candidate{Adapter: "stepik", ExternalID: "54403", URL: "https://stepik.org/course/54403/promo"}
	zero := int64(0)
	for _, title := range []string{"Go course", "Renamed Go course"} {
		record, err := normalizedRecord(candidate, title, "Go curriculum", "Stepik", Observation{Price: &zero, Enrollment: "continuous"}, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		n, err := db.UpdateVerified(ctx, config.Templates.Domains, "test", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
			resolved, err := bindIdentity(ctx, tx, candidate, record, old)
			if err != nil {
				return nil, err
			}
			if resolved.ID != existing.ID || resolved.Offers[0].ID != existing.Offers[0].ID {
				t.Fatal("curated identities replaced")
			}
			code := ""
			return merge(ctx, tx, old, resolved, resolved.ID, Observation{Price: &zero, Enrollment: "continuous"}, time.Now(), &code)
		})
		if err != nil || n != 1 {
			t.Fatalf("publish %d %v", n, err)
		}
	}
	courses, _ := db.Load(ctx)
	if len(courses) != 1 {
		t.Fatal("title change duplicated course")
	}
	var count int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_identities").Scan(&count)
	if count != 1 {
		t.Fatal("duplicate mapping")
	}
}
func TestIdentityAndPublicationRollbackTogether(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	candidate := Candidate{Adapter: "otus", ExternalID: "python-new", URL: "https://otus.ru/lessons/python-new"}
	zero := int64(0)
	record, err := normalizedRecord(candidate, "Python", "Python curriculum", "OTUS", Observation{Price: &zero, Enrollment: "open"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.UpdateVerified(ctx, []string{"otus.ru"}, "test", func(ctx context.Context, tx pgx.Tx, old []catalog.Course) ([]catalog.Course, error) {
		resolved, err := bindIdentity(ctx, tx, candidate, record, old)
		if err != nil {
			return nil, err
		}
		resolved.Language = "invalid"
		return []catalog.Course{resolved}, nil
	})
	if err == nil {
		t.Fatal("invalid record published")
	}
	var count int
	db.Pool.QueryRow(ctx, "SELECT count(*) FROM catalog_identities").Scan(&count)
	if count != 0 {
		t.Fatal("identity leaked from failed publication")
	}
}
