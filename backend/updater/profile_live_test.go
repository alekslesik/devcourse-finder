package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

type profileCoverage struct {
	Provider         string `json:"provider"`
	URL              string `json:"url"`
	Code             string `json:"code"`
	Language         string `json:"language,omitempty"`
	Prerequisites    int    `json:"prerequisites"`
	Audience         int    `json:"audience"`
	Goals            int    `json:"goals"`
	Direction        int    `json:"direction"`
	ExcludedAudience int    `json:"excluded_audience"`
	TargetLevel      int    `json:"target_level"`
	Published        int    `json:"published"`
}

// One candidate/provider, no sitemap expansion, serial reads, a 45s/provider
// context and 7m outer budget. Writes only to the explicit disposable test DB.
func TestProfileLiveCoverage(t *testing.T) {
	if os.Getenv("PROFILE_LIVE_CHECK") != "1" {
		t.Skip("opt-in live official reads with disposable TEST_DATABASE_URL")
	}
	db := postgresFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
	defer cancel()
	defer func() {
		if raw, err := os.ReadFile("/proc/self/status"); err == nil {
			for _, line := range strings.Split(string(raw), "\n") {
				if strings.HasPrefix(line, "VmHWM:") {
					t.Log("process_peak_rss", line)
				}
			}
		}
	}()
	samples := []struct{ adapter, url string }{
		{"yandex", "https://practicum.yandex.ru/backend-developer"}, {"netology", "https://netology.ru/programs/go"}, {"skillbox", "https://skillbox.ru/course/profession-python"}, {"skillfactory", "https://skillfactory.ru/python-developer"}, {"htmlacademy", "https://htmlacademy.ru/intensive/react"}, {"purpleschool", "https://purpleschool.ru/course/javascript-basics"}, {"skypro", "https://sky.pro/course/programming/python-web-course"}, {"otus", "https://otus.ru/lessons/python-basic"}, {"hexlet", "https://ru.hexlet.io/programs/python"}, {"stepik", "https://stepik.org/course/58852/promo"}, {"javarush", "https://javarush.com/prices"}, {"rsschool", "https://rs.school/courses/javascript"}, {"codebasics", "https://code-basics.com/ru/languages/python"},
	}
	var report []profileCoverage
	for _, sample := range samples {
		row := profileCoverage{Provider: sample.adapter, URL: sample.url}
		c, ok := candidateURL(sample.adapter, sample.url, sample.adapter+"-learning-sample")
		if !ok {
			t.Fatal("invalid sample", sample)
		}
		cfg := productionConfig(t)
		cfg.Sources = nil
		cfg.Discovery = []Feed{{ID: c.FeedID, Adapter: c.Adapter, URL: "https://" + providerHosts[c.Adapter] + "/sitemap.xml"}}
		s := Service{DB: db, Config: cfg, Worker: "publisher"}
		local, stop := context.WithTimeout(ctx, 45*time.Second)
		client := NewClient()
		body, digest, err := fetch(local, client, candidateEndpoint(c))
		var r catalog.Course
		var o Observation
		if err == nil {
			switch c.Adapter {
			case "yandex":
				r, o, digest, err = fetchPracticumGroup(local, client, body, digest, c)
			case "skillfactory":
				r, o, digest, err = fetchSkillfactory(local, client, body, digest, c)
			case "htmlacademy":
				r, o, digest, err = fetchHTMLAcademy(local, client, body, digest, c)
			case "javarush":
				r, o, digest, err = fetchJavaRush(local, client, body, digest, c)
			case "rsschool", "codebasics":
				extraURL := "https://rs.school/courses"
				if c.Adapter == "codebasics" {
					extraURL = "https://code-basics.com/ru"
				}
				extra, ed, e := fetch(local, client, extraURL)
				err = e
				if err == nil {
					digest = fingerprint([]byte(digest + ":" + ed))
					if c.Adapter == "rsschool" {
						r, o, err = collectRSSchool(body, extra, c, time.Now().UTC())
					} else {
						r, o, err = collectCodeBasics(body, extra, c, time.Now().UTC())
					}
				}
			default:
				r, o, err = collectCandidate(body, c, time.Now().UTC())
			}
		}
		stop()
		if err != nil {
			row.Code = rejectionCode(err, "source_unavailable")
		} else {
			row.Code = "verified"
			row.Language = r.Language
			if p := r.Profile; p != nil {
				if p.Prerequisites != nil {
					row.Prerequisites = 1
				}
				if p.Audience != nil {
					row.Audience = 1
				}
				if p.Goals != nil {
					row.Goals = 1
				}
				if p.Direction != nil {
					row.Direction = 1
				}
				if p.ExcludedAudience != nil {
					row.ExcludedAudience = 1
				}
				if p.TargetLevel != nil {
					row.TargetLevel = 1
				}
			}
			if len(catalog.Search([]catalog.Course{r}, catalog.Filter{}, time.Now())) == 0 {
				row.Code = "verified_unavailable"
				report = append(report, row)
				raw, _ := json.Marshal(row)
				t.Log(string(raw))
				continue // Never fabricate historical enrollment to make a closed course publish.
			}
			// Seed the pre-profile version of this exact identity. Publication
			// must backfill it rather than create a second course or tariff.
			legacy := r
			legacy.Profile = nil
			legacy.ProfileOwnership = 0
			legacy.Audience = []string{"unknown"}
			legacy.Goals = []string{"unknown"}
			legacy.Direction = "unknown"
			legacyAt := r.CheckedAt.Add(-500 * time.Millisecond)
			legacy.CheckedAt = legacyAt
			priorObservation := o
			priorObservation.ProfileObserved = false
			priorObservation.Profile = nil
			prior := QueuedObservation{Kind: "discovered", Candidate: c, Record: legacy, Observation: priorObservation, ObservedAt: legacyAt, Digest: digest}
			prior = reobserveLearning(t, prior, legacyAt)
			if err = s.enqueue(ctx, prior); err != nil {
				t.Fatal(c.Adapter, "baseline queue", err)
			}
			initial, e := s.Publish(ctx)
			if e != nil || initial.NewCourses != 1 {
				t.Fatal(c.Adapter, "baseline publication", initial, e)
			}

			err = s.enqueue(ctx, QueuedObservation{Kind: "discovered", Candidate: c, Record: r, Observation: o, ObservedAt: r.CheckedAt, Digest: digest})
			if err != nil {
				row.Code = "queue_rejected"
				t.Errorf("%s verified source rejected by queue: %v", c.Adapter, err)
			} else {
				result, e := s.Publish(ctx)
				if e != nil {
					t.Errorf("%s publication: %v", c.Adapter, e)
					row.Code = "publication_failed"
				} else {
					row.Published = result.Published
					if row.Published != 1 {
						t.Errorf("%s verified refresh was not published", c.Adapter)
					}
					if result.NewCourses != 0 {
						t.Errorf("%s backfill duplicated identity", c.Adapter)
					}
				}
			}
		}
		report = append(report, row)
		raw, _ := json.Marshal(row)
		t.Log(string(raw))
	}
	loaded, err := db.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	matrix := map[string]any{}
	for _, language := range []string{"python", "javascript", "java", "go"} {
		f := catalog.Filter{Language: language}
		facets := catalog.ProfileFacets(loaded, f, time.Now())
		matrix[language] = facets
	}
	rawMatrix, _ := json.Marshal(matrix)
	t.Log("post_backfill_filter_matrix", string(rawMatrix))
	if path := os.Getenv("PROFILE_LIVE_REPORT_PATH"); path != "" {
		raw, _ := json.MarshalIndent(map[string]any{"sources": report, "post_backfill_filter_matrix": matrix, "scope": "One official sample per provider; disposable database, not production backfill counts."}, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
