package updater

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

type practicumFixture struct {
	Profession json.RawMessage `json:"profession"`
	Prices     json.RawMessage `json:"prices"`
	Squads     json.RawMessage `json:"squads"`
}

func practicumData(t *testing.T, slug string) ([]byte, practicumFixture, Candidate) {
	t.Helper()
	raw, err := os.ReadFile("testdata/practicum-" + slug + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var f practicumFixture
	if json.Unmarshal(raw, &f) != nil {
		t.Fatal("fixture")
	}
	page, err := os.ReadFile("testdata/practicum-backend-page.html")
	if err != nil {
		t.Fatal(err)
	}
	// Other language cases reuse the recorded page shape with explicit slug
	// substitution; they are parser tests, not claimed successful live page reads.
	page = []byte(strings.ReplaceAll(string(page), "backend-developer", slug))
	return page, f, Candidate{Adapter: "yandex", ExternalID: slug, URL: practicumOrigin + "/" + slug, FeedID: "yandex-sitemap"}
}
func practicumNow() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }
func TestPracticumFullPaymentAndLanguageContracts(t *testing.T) {
	for _, tc := range []struct {
		slug, language string
		price          int64
	}{{"backend-developer", "python", 13700000}, {"java-developer", "java", 17050000}, {"java-developer-plus", "java", 23250000}, {"go-developer-basic", "go", 15400000}, {"frontend-developer", "javascript", 13300000}} {
		t.Run(tc.slug, func(t *testing.T) {
			page, f, c := practicumData(t, tc.slug)
			record, o, err := collectPracticum(page, f.Profession, f.Prices, f.Squads, c, practicumNow())
			if err != nil || o.Price == nil || *o.Price != tc.price || record.Language != tc.language || record.Offers[0].Free || o.Enrollment != "open" || o.ProductID == "" || o.ProfessionID == "" {
				t.Fatal(record, o, err)
			}
			if tc.slug == "java-developer-plus" && (o.ProductID == o.ProfessionID || record.Offers[0].Name != "Расширенная программа") {
				t.Fatal("different IDs/tariff collapsed")
			}
		})
	}
}
func TestPracticumRejectsUnboundPagesAndEndpoints(t *testing.T) {
	page, f, c := practicumData(t, "backend-developer")
	for _, bad := range []string{"<html>Access denied</html>", strings.ReplaceAll(string(page), `lang="ru"`, `lang="en"`), strings.ReplaceAll(string(page), "/backend-developer/\"", "/other-course/\""), strings.ReplaceAll(string(page), "?slugs=backend-developer", "?slugs=java-developer"), strings.ReplaceAll(string(page), "https://practicum.yandex.ru/api/", "https://evil.example/api/"), string(page) + string(page)} {
		if _, _, err := collectPracticum([]byte(bad), f.Profession, f.Prices, f.Squads, c, practicumNow()); err == nil {
			t.Fatal("bad page accepted")
		}
	}
}
func TestPracticumRejectsPriceAndIdentityChanges(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"subscription", "profession_is_regular_charge", true}, {"missing-charge-flag", "profession_is_regular_charge", nil},
		{"free-intro", "profession_final_price", 0}, {"wrong-full-price", "profession_price", 16000},
		{"wrong-discount", "profession_final_price", 100000}, {"missing-full-price", "profession_price", nil},
		{"invalid-product", "profession_product_id", "unbound"}, {"personal-promocode", "absolute_promocode_discount", 1000},
		{"personal-certificate", "certificates", []any{map[string]any{"amount": 1000}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, f, c := practicumData(t, "backend-developer")
			var pricing map[string]map[string]map[string]any
			json.Unmarshal(f.Prices, &pricing)
			pricing[c.ExternalID]["RUB"][tc.key] = tc.value
			b, _ := json.Marshal(pricing)
			if _, _, err := collectPracticum(page, f.Profession, b, f.Squads, c, practicumNow()); err == nil {
				t.Fatal("invalid price accepted")
			}
		})
	}
	page, f, c := practicumData(t, "backend-developer")
	for _, change := range []struct {
		key   string
		value any
	}{{"slug", "python-developer-plus"}, {"currency", "USD"}, {"landing_path", "/other-course/"}, {"is_demand_test", true}, {"hidden_type", "hidden"}} {
		var programs []map[string]any
		json.Unmarshal(f.Profession, &programs)
		programs[0][change.key] = change.value
		b, _ := json.Marshal(programs)
		if _, _, err := collectPracticum(page, b, f.Prices, f.Squads, c, practicumNow()); err == nil {
			t.Fatal(change.key)
		}
	}
}
func TestPracticumDiscountDeadlineAndCohortAvailability(t *testing.T) {
	page, f, c := practicumData(t, "backend-developer")
	var pricing map[string]map[string]map[string]any
	json.Unmarshal(f.Prices, &pricing)
	p := pricing[c.ExternalID]["RUB"]
	p["profession_final_price"] = 123300
	p["percent_discount"] = 10
	p["total_absolute_discount"] = 13700
	p["discount_deadline"] = "2026-10-10T21:00:00Z"
	b, _ := json.Marshal(pricing)
	_, o, err := collectPracticum(page, f.Profession, b, f.Squads, c, practicumNow())
	if err != nil || *o.Price != 12330000 || o.ValidUntil == nil || o.ValidUntil.Format(time.RFC3339) != "2026-10-10T21:00:00Z" {
		t.Fatal(o, err)
	}
	if _, _, err = collectPracticum(page, f.Profession, b, f.Squads, c, practicumNow().Add(48*time.Hour)); err == nil {
		t.Fatal("expired discount accepted")
	}
	_, o, err = collectPracticum(page, f.Profession, f.Prices, []byte(`[]`), c, practicumNow())
	if err != nil || o.Enrollment != "closed" {
		t.Fatal(o, err)
	}
	for _, bad := range []string{`null`, `[{"id":1,"name":"another_cohort_1"}]`} {
		if _, _, err = collectPracticum(page, f.Profession, f.Prices, []byte(bad), c, practicumNow()); err == nil {
			t.Fatal("invalid cohort accepted")
		}
	}
	var cohorts []map[string]any
	json.Unmarshal(f.Squads, &cohorts)
	for _, cohort := range cohorts {
		cohort["subscriptions_num"] = cohort["subscriptions_limit"]
	}
	b, _ = json.Marshal(cohorts)
	_, o, err = collectPracticum(page, f.Profession, f.Prices, b, c, practicumNow())
	if err != nil || o.Enrollment != "closed" {
		t.Fatal("full cohorts advertised", o, err)
	}
}
func TestPracticumOfficialExtensionlessSitemapOnly(t *testing.T) {
	if err := validateFeed(Feed{ID: "yandex", Adapter: "yandex", URL: practicumOrigin + "/lang-static/sitemap/"}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/lang-static/other/", "/api/v3/professions/prices/", "/sitemap"} {
		if validateFeed(Feed{ID: "yandex", Adapter: "yandex", URL: practicumOrigin + path}) == nil {
			t.Fatal(path)
		}
	}
}
