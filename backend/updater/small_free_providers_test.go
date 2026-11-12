package updater

import (
	"bytes"
	"context"
	"devcourse-finder/catalog"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func smallFreeFixture(t *testing.T, adapter string) ([]byte, Candidate) {
	t.Helper()
	file := "codechick-python"
	source := "https://codechick.io/paths/python-s-nulya"
	if adapter == "learnpython" {
		file = "learnpython"
		source = "https://www.learnpython.org/ru"
	}
	raw, err := os.ReadFile("testdata/small-free/" + file + ".html")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := candidateURL(adapter, source, adapter+"-catalog")
	if !ok {
		t.Fatal(source)
	}
	return raw, c
}

func TestSmallFreeProgramsSourceIdentityAndUnknownConditions(t *testing.T) {
	for _, adapter := range []string{"codechick", "learnpython"} {
		t.Run(adapter, func(t *testing.T) {
			raw, c := smallFreeFixture(t, adapter)
			now := time.Now().UTC()
			r, o, err := collectCandidate(raw, c, now)
			if err != nil || r.Provider != providerNames[adapter] || r.Language != "python" || len(r.Offers) != 1 || !r.Offers[0].Free || *r.Offers[0].Price != 0 || r.Offers[0].Enrollment != "continuous" || r.Offers[0].Schedule != "flexible" {
				t.Fatal(r, o, err)
			}
			if r.Offers[0].Weeks != nil || r.Offers[0].Hours != nil || r.Offers[0].DurationMonths != nil || r.Offers[0].SupportKnown == nil || *r.Offers[0].SupportKnown || r.Offers[0].LearningConditions != nil {
				t.Fatal("invented calendar, weekly effort, approximate total or teacher support", r.Offers)
			}
			if adapter == "codechick" && (r.Profile == nil || r.Profile.Audience == nil || r.Profile.Audience.Values[0] != "none" || !r.Profile.Valid()) {
				t.Fatal("explicit audience lost", r.Profile)
			}
			if o.ValidUntil == nil || o.ValidUntil.Sub(now) != 26*time.Hour {
				t.Fatal("missing bounded free-price lease", o)
			}
			if _, ok := aiCandidateURL(c.URL); !ok {
				t.Fatal("reader not registered")
			}
		})
	}
}

func TestCodeChickRejectsIntroOnlyForeignAndAmbiguousContracts(t *testing.T) {
	raw, c := smallFreeFixture(t, "codechick")
	for _, pair := range [][2]string{
		{`"price":"0"`, `"price":"1000"`}, {`"isAccessibleForFree":true`, `"isAccessibleForFree":false`},
		{`"name":"CodeChick"`, `"name":"Other School"`},
		{`"url":"https://codechick.io/paths/python-s-nulya"`, `"url":"https://codechick.io/paths/other"`},
		{`Все уроки, упражнения и экзамен бесплатны — без скрытых платежей и подписок.`, `Только первый урок бесплатный, далее платная подписка.`},
		{`навсегда, без подписок`, `только первый урок`}, {`<html lang="ru">`, `<html lang="en">`},
	} {
		bad := strings.Replace(string(raw), pair[0], pair[1], 1)
		if bad == string(raw) {
			t.Fatal("fixture mutation absent", pair[0])
		}
		if _, _, err := collectCandidate([]byte(bad), c, time.Now().UTC()); err == nil {
			t.Fatal("unsafe free program accepted", pair[0])
		}
	}
	if cleaned, ok := candidateURL("codechick", c.URL+"?price=0", ""); !ok || cleaned.URL != c.URL {
		t.Fatal("tracking was not removed before fetching")
	}
	if _, ok := aiCandidateURL(c.URL + "?price=0"); ok {
		t.Fatal("noncanonical AI source accepted")
	}
	if _, _, err := collectCandidate(append(raw, raw...), c, time.Now().UTC()); err == nil {
		t.Fatal("duplicate course accepted")
	}
	for _, source := range []string{"https://codechick.io/paths/python-s-nulya/lesson", "https://codechick.io.evil.example/paths/python-s-nulya"} {
		if _, ok := candidateURL("codechick", source, ""); ok {
			t.Fatal(source)
		}
	}
}

func TestLearnPythonScopesOnlyLocalCurriculumAndRejectsForeignSections(t *testing.T) {
	raw, c := smallFreeFixture(t, "learnpython")
	_, text, err := learnPythonScope(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, excluded := range []string{"DataCamp", "codingforkids", "Другие учебники", "LearnX", "Facebook", "сертифицироваться"} {
		if strings.Contains(text, excluded) {
			t.Fatal("sponsor/other product leaked", excluded, text)
		}
	}
	for _, supported := range []string{"Переменные и типы", "Функции", "Декораторы"} {
		if !strings.Contains(text, supported) {
			t.Fatal("curriculum lost", text)
		}
	}
	for _, pair := range [][2]string{{`is a free interactive Python tutorial`, `is a paid Python course`}, {`href="Lists"`, `href="https://other.example/course"`}, {`<h3>Изучите основы</h3>`, `<h3>Другая программа</h3>`}, {`lang="ru"`, `lang="en"`}} {
		bad := strings.Replace(string(raw), pair[0], pair[1], 1)
		if bad == string(raw) {
			t.Fatal("fixture mutation absent", pair[0])
		}
		if _, _, err := collectCandidate([]byte(bad), c, time.Now().UTC()); err == nil {
			t.Fatal("foreign/incomplete tutorial accepted", pair[0])
		}
	}
	for _, source := range []string{"https://www.learnpython.org/en", "https://www.learnpython.org/ru/Lists", "https://learnpython.org/ru", "http://www.learnpython.org/ru"} {
		if _, ok := candidateURL("learnpython", source, ""); ok {
			t.Fatal(source)
		}
	}
}

func TestSmallFreeProvidersDiscoverQueuePublishAndBindAIWithoutSpending(t *testing.T) {
	for _, adapter := range []string{"codechick", "learnpython"} {
		t.Run(adapter, func(t *testing.T) {
			db := postgresFixture(t)
			ctx := context.Background()
			raw, c := smallFreeFixture(t, adapter)
			config := productionConfig(t)
			config.Sources = nil
			root := "https://codechick.io/"
			if adapter == "learnpython" {
				root = "https://www.learnpython.org/ru/"
			}
			config.Discovery = []Feed{{ID: c.FeedID, Adapter: adapter, URL: root, Kind: "catalog"}}
			if err := validateFeed(config.Discovery[0]); err != nil {
				t.Fatal(err)
			}
			client := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
				body := raw
				if req.URL.String() == root && adapter == "codechick" {
					body = []byte(`<a href="` + c.URL + `">Python</a>`)
				}
				if req.URL.String() != root && req.URL.String() != candidateEndpoint(c) {
					t.Fatal("foreign fetch", req.URL)
				}
				// The LearnPython root and detail share a URL; its real language switcher
				// includes this own-root link. Keep curriculum intact in the fixture.
				if adapter == "learnpython" {
					body = append(append([]byte(nil), body...), []byte(`<a href="/ru/">Русский</a>`)...)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body))}, nil
			})}
			// Discovery is separate from page collection; both use the normal durable queue.
			discovery := Service{DB: db, Config: config, Worker: "pages", Client: client}
			if _, err := discovery.discover(ctx, client); err != nil {
				t.Fatal(err)
			}
			pages := Service{DB: db, Config: config, Worker: "pages", Client: client}
			if result, err := pages.Run(ctx); err != nil || result.Queued != 1 {
				t.Fatal(result, err)
			}
			if records, err := db.Load(ctx); err != nil || len(records) != 0 {
				t.Fatal("collector published directly", records, err)
			}
			publisher := Service{DB: db, Config: config, Worker: "publisher"}
			if result, err := publisher.Run(ctx); err != nil || result.Published != 1 {
				t.Fatal(result, err)
			}
			records, err := db.Load(ctx)
			if err != nil || len(records) != 1 {
				t.Fatal(records, err)
			}
			before := aiProtectedDigest(records[0])
			candidate, docs, err := aiFetchDocuments(ctx, db, client, records[0])
			if err != nil || len(docs) != 1 || docs[0].Scope != "course" || docs[0].BindingDigest == "" || candidate.Adapter != adapter {
				t.Fatal(docs, err)
			}
			if !aiDocumentsValid(candidate, docs) {
				t.Fatal("unbound AI curriculum")
			}
			var reservations int
			if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM catalog_ai_daily_budget`).Scan(&reservations); err != nil || reservations != 0 {
				t.Fatal("source-only integration spent AI budget", reservations, err)
			}
			after, _ := db.Load(ctx)
			if aiProtectedDigest(after[0]) != before {
				t.Fatal("source preparation changed public facts")
			}
			if _, err = db.Pool.Exec(ctx, `DELETE FROM catalog_identities WHERE adapter=$1`, adapter); err != nil {
				t.Fatal(err)
			}
			if _, _, err = aiFetchDocuments(ctx, db, client, after[0]); err == nil {
				t.Fatal("AI bypassed saved identity")
			}
		})
	}
}

func TestNewFreeProgramsParticipateInCatalogFiltersAndSchoolOrdering(t *testing.T) {
	now := time.Now().UTC()
	courses := []catalog.Course{}
	for _, adapter := range []string{"codechick", "learnpython"} {
		raw, c := smallFreeFixture(t, adapter)
		r, _, err := collectCandidate(raw, c, now)
		if err != nil {
			t.Fatal(err)
		}
		courses = append(courses, r)
	}
	// Ranking accepts newly connected providers without a priority-list change.
	results := catalog.Search(courses, catalog.Filter{Language: "python", IncludeFree: true, Sort: "discovery", Page: 1, PageSize: 1}, now)
	if len(results) != 2 || results[0].Course.Provider == results[1].Course.Provider {
		t.Fatal("new schools disappeared", results)
	}
	second := catalog.Search(courses, catalog.Filter{Language: "python", IncludeFree: true, Sort: "discovery", Page: 2, PageSize: 1}, now)
	if !reflect.DeepEqual(results, second) {
		t.Fatal("page changed school ranking")
	}
	beginner := catalog.Search(courses, catalog.Filter{Language: "python", Experience: "none", IncludeFree: true}, now)
	if len(beginner) != 1 || beginner[0].Course.Provider != "CodeChick" {
		t.Fatal("supported beginner suitability lost, or unknown suitability invented", beginner)
	}
	filtered := catalog.Search(courses, catalog.Filter{Language: "java", IncludeFree: true}, now)
	if len(filtered) != 0 {
		t.Fatal("school diversity bypassed hard filters")
	}
}
