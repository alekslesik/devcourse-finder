package updater

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func purpleFixture(t *testing.T, slug string) ([]byte, Candidate) {
	t.Helper()
	b, err := os.ReadFile("testdata/purpleschool-" + slug + ".html")
	if err != nil {
		t.Fatal(err)
	}
	return b, Candidate{Adapter: "purpleschool", ExternalID: slug, URL: "https://purpleschool.ru/course/" + slug, FeedID: "purpleschool-sitemap"}
}
func TestPurpleSchoolRecordedTariffs(t *testing.T) {
	for _, slug := range []string{"go-basics", "javascript-basics", "python-start"} {
		t.Run(slug, func(t *testing.T) {
			b, c := purpleFixture(t, slug)
			r, o, err := collectPurpleSchool(b, c, netologyNow())
			if err != nil {
				t.Fatal(err)
			}
			if o.PurpleCourseID <= 0 || len(r.Offers) != len(o.PurpleTariffs) {
				t.Fatal(r, o)
			}
			if slug == "python-start" {
				if len(r.Offers) != 1 || !r.Offers[0].Free {
					t.Fatal(r)
				}
			} else {
				if len(r.Offers) != 3 || *r.Offers[0].Price != 399900 || *r.Offers[1].Price != 549900 || *r.Offers[2].Price != 1299900 || r.Offers[1].Mentor || !r.Offers[2].Mentor || !r.Offers[2].Review {
					t.Fatal(r)
				}
			}
		})
	}
}
func purpleMutate(t *testing.T, b []byte, change func(map[string]any)) []byte {
	t.Helper()
	start := strings.Index(string(b), "<script>self.__next_f.push(") + len("<script>self.__next_f.push(")
	end := strings.Index(string(b[start:]), ")</script>") + start
	var parts []json.RawMessage
	if err := json.Unmarshal(b[start:end], &parts); err != nil {
		t.Fatal(err)
	}
	var stream string
	json.Unmarshal(parts[1], &stream)
	_, raw, _ := strings.Cut(stream, ":")
	var record []any
	json.Unmarshal([]byte(raw), &record)
	change(record[3].(map[string]any)["course"].(map[string]any))
	next, _ := json.Marshal(record)
	chunk, _ := json.Marshal([]any{1, "1:" + string(next) + "\n"})
	return []byte(string(b[:start]) + string(chunk) + string(b[end:]))
}
func TestPurpleSchoolRejectsUnboundOrStalePrices(t *testing.T) {
	b, c := purpleFixture(t, "go-basics")
	for _, bad := range []string{
		"<html>challenge</html>",
		strings.ReplaceAll(string(b), "3 999 ₽", "1 999 ₽"),
		strings.ReplaceAll(string(b), "tariffId=50", "tariffId=999"),
		strings.ReplaceAll(string(b), "buy?course=19", "buy?course=20"),
		strings.ReplaceAll(string(b), "app.purpleschool.ru", "evil.example"),
		strings.ReplaceAll(string(b), "priceCurrency", "billingDuration"),
		strings.ReplaceAll(string(b), `"priceSpecification":[{"@type":"PriceSpecification"`, `"priceSpecification":[{"billingIncrement":1,"@type":"PriceSpecification"`),
		strings.ReplaceAll(string(b), `"availability":"https://schema.org/InStock"`, `"priceValidUntil":"2023-01-01T00:00:00Z","availability":"https://schema.org/InStock"`),
		strings.ReplaceAll(string(b), ">Начать курс</button>", ">Недоступно</button>"),
		strings.ReplaceAll(string(b), "Самостоятельный", "Демо модули"),
		strings.ReplaceAll(string(b), "TariffCardV2-module_", "Unknown-module_"),
	} {
		if _, _, err := collectPurpleSchool([]byte(bad), c, netologyNow()); err == nil {
			t.Fatal("unsafe source accepted")
		}
	}
	for _, change := range []func(map[string]any){
		func(p map[string]any) { p["isOnSite"] = false },
		func(p map[string]any) { p["plannedReleaseDate"] = "2030-01-01" },
		func(p map[string]any) { p["status"] = "draft" },
		func(p map[string]any) { p["tariffs"].([]any)[1].(map[string]any)["price"] = 0 },
		func(p map[string]any) { p["tariffs"].([]any)[1].(map[string]any)["courseId"] = 20 },
		func(p map[string]any) { p["tariffs"].([]any)[1].(map[string]any)["status"] = "INACTIVE" },
		func(p map[string]any) { p["tariffs"].([]any)[1].(map[string]any)["tariffInSubscription"] = []any{1} },
		func(p map[string]any) { p["tariffs"].([]any)[3].(map[string]any)["features"] = []any{} },
	} {
		bad := purpleMutate(t, b, change)
		if _, _, err := collectPurpleSchool(bad, c, netologyNow()); err == nil {
			t.Fatal("unsafe contract accepted")
		}
	}
}
func TestPurpleSchoolFreeRequiresCompleteCurriculum(t *testing.T) {
	b, c := purpleFixture(t, "python-start")
	for _, bad := range [][]byte{[]byte(strings.ReplaceAll(string(b), "Полный курс - Бесплатно", "Первые уроки бесплатно")), purpleMutate(t, b, func(p map[string]any) {
		p["sections"].([]any)[0].(map[string]any)["lessons"].([]any)[0].(map[string]any)["lessonOnTariff"] = []any{}
	})} {
		if _, _, err := collectPurpleSchool(bad, c, netologyNow()); err == nil {
			t.Fatal("partial free access accepted")
		}
	}
}
func TestPurpleSchoolLiveVerification(t *testing.T) {
	if os.Getenv("PURPLESCHOOL_LIVE_CHECK") != "1" {
		t.Skip("opt-in bounded official reads")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	for _, slug := range []string{"go-basics", "javascript-basics", "python-start"} {
		_, c := purpleFixture(t, slug)
		b, _, err := fetch(ctx, NewClient(), c.URL)
		if err != nil {
			t.Fatal(err)
		}
		r, _, err := collectPurpleSchool(b, c, time.Now().UTC())
		if err != nil {
			t.Fatal(slug, err)
		}
		t.Log(slug, len(r.Offers), "verified tariffs")
	}
}
