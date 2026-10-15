package updater

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func recordedPurpleProfile(t *testing.T) purpleCourse {
	t.Helper()
	raw, err := os.ReadFile("testdata/profiles/purpleschool-python-start-2026-10-15.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Course purpleCourse `json:"course"`
	}
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot.Course
}

func TestPurpleRecordedProfileBackfillsSameFrozenCourse(t *testing.T) {
	course := recordedPurpleProfile(t)
	c := Candidate{Adapter: "purpleschool", ExternalID: course.Alias, URL: "https://purpleschool.ru/course/" + course.Alias}
	now := time.Date(2026, 10, 15, 8, 0, 0, 0, time.UTC)
	raw, _ := json.Marshal(course)
	p := purpleProfile(raw, course, c, "python", now)
	if !p.Valid() || p.Prerequisites == nil || p.Prerequisites.Level != "none" || p.Audience == nil || p.Goals == nil || p.Goals.Values[0] != "try" {
		t.Fatalf("recorded facts lost: %+v", p)
	}
	for _, e := range p.Evidence() {
		if e.Source != c.URL || !e.CheckedAt.Equal(now) || e.Digest != fingerprint(raw) || !strings.HasPrefix(e.Locator, "course[alias=python-start].") {
			t.Fatal("provenance", e)
		}
	}
	// Same course, offer, clock and filters; only observed profile fields change.
	zero := int64(0)
	legacy := catalog.Course{ID: "purple-frozen", Status: "published", Language: "python", Audience: []string{"unknown"}, Goals: []string{"unknown"}, Offers: []catalog.Offer{{ID: "purple-frozen-free", Free: true, Price: &zero, PriceKind: "exact", PriceCheckedAt: now, Enrollment: "continuous", Schedule: "flexible"}}}
	f := catalog.Filter{Language: "python", Experience: "none", Goal: "try", IncludeUnknownProfiles: true}
	before := catalog.Search([]catalog.Course{legacy}, f, now)
	updated := legacy
	applyCourseProfile(&updated, p)
	after := catalog.Search([]catalog.Course{updated}, f, now)
	if len(before) != 1 || len(after) != 1 || catalog.MatchProfile(legacy, "goal", "try") != "unknown" || catalog.MatchProfile(updated, "goal", "try") != "confirmed" || catalog.MatchProfile(updated, "experience", "none") != "confirmed" || updated.ID != legacy.ID || updated.Offers[0].ID != legacy.Offers[0].ID {
		t.Fatal("same-snapshot backfill changed identity or lost matching")
	}
	t.Log("fixed one-course snapshot: confirmed 0 -> 1; unverified 1 -> 0; total 1 -> 1")
}

func TestPurpleProfileDoesNotInferFromTitlesRequirementsOrMarketing(t *testing.T) {
	course := recordedPurpleProfile(t)
	c := Candidate{Adapter: "purpleschool", ExternalID: course.Alias, URL: "https://purpleschool.ru/course/" + course.Alias}
	for _, description := range []string{"", "<p>Python с нуля для первой работы</p>", "<p>Не для новичков. Например, познакомиться с Python и попробовать его на практике.</p>", "<p>Если вы уже знаете основы, этот курс расчитан на тех, что хочет познакомиться с Python и попробовать его на практике</p>"} {
		course.Description = description
		course.Requirements = []string{"Требований нет, кроме базовых знаний Python"}
		p := purpleProfile([]byte(description), course, c, "python", time.Now().UTC())
		if p.Goals != nil && p.Goals.Values[0] == "try" {
			t.Fatal("invented purpose", description)
		}
		if p.Prerequisites != nil && p.Prerequisites.Level == "none" {
			t.Fatal("qualified requirement became no requirements")
		}
	}
	course = recordedPurpleProfile(t)
	course.Description = "<p>Требования: необходимы базовые знания Python.</p>"
	p := purpleProfile([]byte(course.Description), course, c, "python", time.Now().UTC())
	if p.Audience != nil || p.Goals != nil {
		t.Fatal("no requirements does not mean audience or purpose")
	}
}

func TestPurpleProfileIsAttachedOnlyToVerifiedIdentity(t *testing.T) {
	raw, c := purpleFixture(t, "python-start")
	observed := recordedPurpleProfile(t)
	// Synthetic envelope mutation around an independently recorded field excerpt.
	raw = purpleMutate(t, raw, func(p map[string]any) {
		p["description"] = observed.Description
		p["requirements"] = observed.Requirements
	})
	r, o, err := collectPurpleSchool(raw, c, netologyNow())
	if err != nil || r.Profile == nil || r.Profile.Goals == nil || !o.ProfileObserved {
		t.Fatal("verified profile not attached", err)
	}
	c.ExternalID = "unrelated-course"
	if _, _, err = collectPurpleSchool(raw, c, netologyNow()); err == nil {
		t.Fatal("unrelated identity accepted")
	}
}

func TestPurpleBackfillCountsOnFrozenPublicProfileSnapshot(t *testing.T) {
	raw, err := os.ReadFile("testdata/profiles/python-try-snapshot-2026-10-15.json")
	if err != nil {
		t.Fatal(err)
	}
	var snapshot struct {
		Courses []catalog.Course `json:"courses"`
	}
	if err = json.Unmarshal(raw, &snapshot); err != nil {
		t.Fatal(err)
	}
	count := func() (confirmed, unknown, incompatible int) {
		for _, course := range snapshot.Courses {
			a, g := catalog.MatchProfile(course, "experience", "none"), catalog.MatchProfile(course, "goal", "try")
			if a == "incompatible" || g == "incompatible" {
				incompatible++
			} else if a == "unknown" || g == "unknown" {
				unknown++
			} else {
				confirmed++
			}
		}
		return
	}
	if c, u, x := count(); c != 3 || u != 83 || x != 0 {
		t.Fatal("baseline drift", c, u, x)
	}
	course := recordedPurpleProfile(t)
	candidate := Candidate{Adapter: "purpleschool", ExternalID: course.Alias, URL: "https://purpleschool.ru/course/" + course.Alias}
	raw, _ = json.Marshal(course)
	p := purpleProfile(raw, course, candidate, "python", time.Now().UTC())
	changed := 0
	for i := range snapshot.Courses {
		if snapshot.Courses[i].ID == "purpleschool-python-start" {
			applyCourseProfile(&snapshot.Courses[i], p)
			changed++
		}
	}
	if c, u, x := count(); changed != 1 || c != 4 || u != 82 || x != 0 {
		t.Fatal("backfill counts or unrelated facts changed", changed, c, u, x)
	}
	t.Log("fixed 86-course API projection: confirmed 3 -> 4; unverified 83 -> 82; incompatible 0 -> 0; no added identities")
}
