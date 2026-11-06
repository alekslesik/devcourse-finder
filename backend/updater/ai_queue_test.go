package updater

import (
	"context"
	"reflect"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestAIMissingProfileUsesSearchSemantics(t *testing.T) {
	c := catalog.Course{Direction: "backend", Audience: []string{"unknown"}, Goals: []string{"job"}, Profile: &catalog.CourseProfile{Prerequisites: &catalog.ProfileRequirement{Level: "none", Scope: "programming"}}}
	if got := aiProfileMissing(c); !got["experience"] || got["goal"] || got["direction"] {
		t.Fatal(got)
	}
	c.Audience = []string{"none"}
	if aiMissingCount(c) != 0 {
		t.Fatal(aiProfileMissing(c))
	}
	c.Profile.Direction = &catalog.ProfileValue{Value: "unknown"}
	if !aiProfileMissing(c)["direction"] {
		t.Fatal("invalid profile fell back to legacy direction")
	}
}

func TestAIQueuePrioritizesFilterGapsWithoutBypassingEligibility(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	for i, id := range []string{"a-complete-new", "b-missing-old", "c-missing-new", "d-complete-old", "e-not-due", "f-demo", "g-draft"} {
		audience, goals, direction := []string{"unknown"}, []string{"unknown"}, "unknown"
		if id == "a-complete-new" || id == "d-complete-old" {
			audience = []string{"none"}
			goals = []string{"try"}
			direction = "basics"
		}
		state := "published"
		if id == "g-draft" {
			state = "archived"
		}
		_, err := db.Pool.Exec(ctx, `INSERT INTO courses(id,slug,title,provider,language,direction,summary,audience,goals,topics,source,checked_at,status,demo) VALUES($1,$1,$1,'Stepik','python',$2,'Summary',$3,$4,'{}',$5,now(),$6,$7)`, id, direction, audience, goals, "https://stepik.org/course/"+string(rune('1'+i))+"/promo", state, id == "f-demo")
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"b-missing-old", "d-complete-old", "e-not-due"} {
		next := time.Now().Add(-time.Hour)
		if id == "e-not-due" {
			next = time.Now().Add(time.Hour)
		}
		if _, err := db.Pool.Exec(ctx, `INSERT INTO catalog_ai_sources(course_id,attempted_at,next_attempt_at) VALUES($1,now()-interval '1 day',$2)`, id, next); err != nil {
			t.Fatal(err)
		}
	}
	s := AIService{DB: db, Config: AIConfig{BatchSize: 3}}
	ids, err := s.selectAICourses(ctx, true)
	if err != nil || !reflect.DeepEqual(ids, []string{"c-missing-new", "a-complete-new", "b-missing-old"}) {
		t.Fatal(ids, err)
	}
	ids, err = s.selectAICourses(ctx, false)
	if err != nil || len(ids) != 0 {
		t.Fatal("paused provider consumed batch", ids, err)
	}
	s.Config.CourseIDs = []string{"e-not-due"}
	ids, err = s.selectAICourses(ctx, true)
	if err != nil || len(ids) != 0 {
		t.Fatal("manual selection bypassed due time", ids, err)
	}
	s.TargetCourseID = "e-not-due"
	ids, err = s.selectAICourses(ctx, true)
	if err != nil || !reflect.DeepEqual(ids, []string{"e-not-due"}) {
		t.Fatal("explicit verification lost its target", ids, err)
	}
	coverage, err := aiCoverage(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range coverage {
		if v.Provider == "stepik" {
			if v.Eligible != 5 || v.NeverAttempted != 2 || v.Due != 4 || v.MissingProfile != 3 || v.ProfileUnknown["goal"] != 3 {
				t.Fatal(v)
			}
		}
	}
}
