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

type learningCoverage struct {
	Provider  string `json:"provider"`
	URL       string `json:"url"`
	Code      string `json:"code"`
	Offers    int    `json:"offers"`
	Duration  int    `json:"duration"`
	Workload  int    `json:"weekly_workload"`
	Review    int    `json:"review"`
	Mentor    int    `json:"mentor"`
	Community int    `json:"community"`
	Published int    `json:"published"`
}

// One candidate/provider, no sitemap expansion, serial reads, a 45s/provider
// context and 7m outer budget. Writes only to the explicit disposable test DB.
func TestLearningLiveCoverage(t *testing.T) {
	if os.Getenv("LEARNING_LIVE_CHECK") != "1" {
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
	var report []learningCoverage
	for _, sample := range samples {
		row := learningCoverage{Provider: sample.adapter, URL: sample.url}
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
			row.Offers = len(r.Offers)
			for _, offer := range r.Offers {
				v := offer.LearningConditions
				if v == nil {
					continue
				}
				if v.Duration != nil {
					row.Duration++
				}
				if v.WeeklyWorkload != nil {
					row.Workload++
				}
				if v.Review != nil {
					row.Review++
				}
				if v.Mentor != nil {
					row.Mentor++
				}
				if v.Community != nil {
					row.Community++
				}
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
				}
			}
		}
		report = append(report, row)
		raw, _ := json.Marshal(row)
		t.Log(string(raw))
	}
	if path := os.Getenv("LEARNING_LIVE_REPORT_PATH"); path != "" {
		raw, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
