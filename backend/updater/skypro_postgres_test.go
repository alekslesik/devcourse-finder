package updater

import (
	"context"
	"devcourse-finder/catalog"
	"testing"
	"time"
)

func TestSkyproClosedCourseDoesNotPublishAndUnknownNeverMatchesBudget(t *testing.T) {
	db := postgresFixture(t)
	b, c := skyFixture(t, "python")
	r, o, e := collectSkypro(b, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	svc := Service{DB: db}
	code, n, e := svc.publishDiscovered(context.Background(), c, r, o, fingerprint(b))
	if e != nil || n != 0 || code != "insufficient_initial_evidence" {
		t.Fatal(code, n, e)
	}
	max := int64(999999999)
	r.Offers[0].Enrollment = "open"
	if rows := catalog.Search([]catalog.Course{r}, catalog.Filter{Max: &max}, time.Now()); len(rows) != 0 {
		t.Fatal("unknown full price matched budget", rows)
	}
}
