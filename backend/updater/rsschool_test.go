package updater

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func rsFixture(t *testing.T, slug string) ([]byte, []byte, Candidate) {
	t.Helper()
	b, e := os.ReadFile("testdata/rsschool-" + slug + ".html")
	if e != nil {
		t.Fatal(e)
	}
	index, e := os.ReadFile("testdata/rsschool-courses.html")
	if e != nil {
		t.Fatal(e)
	}
	return b, index, Candidate{Adapter: "rsschool", ExternalID: slug, URL: "https://rs.school/courses/" + slug, FeedID: "rsschool-catalog"}
}
func TestRSSchoolRecordedRussianEnrollment(t *testing.T) {
	b, index, c := rsFixture(t, "javascript")
	for _, now := range []time.Time{time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), netologyNow()} {
		r, o, e := collectRSSchool(b, index, c, now)
		if e != nil || o.Price == nil || *o.Price != 0 || !r.Offers[0].Free || r.Language != "javascript" || r.Offers[0].Mentor || r.Offers[0].Review {
			t.Fatal(r, o, e)
		}
		expected := "closed"
		if now.Month() == 9 {
			expected = "open"
		}
		if o.Enrollment != expected {
			t.Fatal(o)
		}
	}
	b, index, c = rsFixture(t, "javascript-preschool-ru")
	if _, _, e := collectRSSchool(b, index, c, netologyNow()); rejectionCode(e, "") != "unverified_enrollment" {
		t.Fatal("TBD enrollment accepted", e)
	}
	b, index, c = rsFixture(t, "reactjs")
	if _, _, e := collectRSSchool(b, index, c, netologyNow()); rejectionCode(e, "") != "unsupported_content_language" {
		t.Fatal("English-only cohort accepted", e)
	}
}
func TestRSSchoolRejectsMissingOrContradictoryEvidence(t *testing.T) {
	b, index, c := rsFixture(t, "javascript")
	for _, bad := range []string{"<html>archived materials</html>", strings.ReplaceAll(string(b), "Our training is completely free", "Our first lesson is free"), strings.ReplaceAll(string(b), "2026-09-27T23:59:59.999Z", "2026-10-27T23:59:59.999Z"), strings.ReplaceAll(string(b), "2026q3", "2025q3"), strings.ReplaceAll(string(b), "2026q3", "2026q4"), strings.ReplaceAll(string(b), "English, Русский", "English"), strings.ReplaceAll(string(b), "https://github.com/rolling-scopes-school/js-fe-course-en", "https://github.com/unrelated/course")} {
		if _, _, e := collectRSSchool([]byte(bad), index, c, netologyNow()); e == nil {
			t.Fatal("invalid evidence accepted")
		}
	}
	if _, _, e := collectRSSchool(b, []byte("<html>stale index</html>"), c, netologyNow()); e == nil {
		t.Fatal("missing independent catalog accepted")
	}
}
func TestRSSchoolLiveVerification(t *testing.T) {
	if os.Getenv("RSSCHOOL_LIVE_CHECK") != "1" {
		t.Skip("opt-in official reads")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	_, _, c := rsFixture(t, "javascript")
	b, _, e := fetch(ctx, NewClient(), c.URL)
	if e != nil {
		t.Fatal(e)
	}
	index, _, e := fetch(ctx, NewClient(), "https://rs.school/courses")
	if e != nil {
		t.Fatal(e)
	}
	r, o, e := collectRSSchool(b, index, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	t.Log(r.Title, o.Enrollment, *o.Price)
}
