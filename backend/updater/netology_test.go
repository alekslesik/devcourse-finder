package updater

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func netologyFixture(t *testing.T, slug string) ([]byte, Candidate) {
	t.Helper()
	data, err := os.ReadFile("testdata/netology-" + slug + ".html")
	if err != nil {
		t.Fatal(err)
	}
	return data, Candidate{Adapter: "netology", ExternalID: slug, URL: "https://netology.ru/programs/" + slug, FeedID: "netology-catalog"}
}
func netologyNow() time.Time { return time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC) }
func TestNetologyRecordedFullPayments(t *testing.T) {
	for _, tc := range []struct {
		slug, language string
		price          int64
	}{{"java-developer", "java", 13170000}, {"go", "go", 9990000}} {
		t.Run(tc.slug, func(t *testing.T) {
			data, c := netologyFixture(t, tc.slug)
			record, o, err := collectNetology(data, c, netologyNow())
			if err != nil || o.Price == nil || *o.Price != tc.price || record.Language != tc.language || o.Enrollment != "open" || o.NetologyFamilyID <= 0 || o.NetologyProgramID <= 0 {
				t.Fatal(record, o, err)
			}
		})
	}
	data, c := netologyFixture(t, "python")
	if _, _, err := collectNetology(data, c, netologyNow()); rejectionCode(err, "") != "unverified_enrollment" {
		t.Fatal("conflicting start dates accepted", err)
	}
}
func netologyMutate(t *testing.T, data []byte, change func(map[string]any)) []byte {
	t.Helper()
	start := strings.Index(string(data), `<script id="__NEXT_DATA__" type="application/json">`) + len(`<script id="__NEXT_DATA__" type="application/json">`)
	end := strings.Index(string(data[start:]), "</script>") + start
	var d map[string]any
	if err := json.Unmarshal(data[start:end], &d); err != nil {
		t.Fatal(err)
	}
	change(d)
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(string(data[:start]) + string(raw) + string(data[end:]))
}
func netologyProgramMap(d map[string]any) map[string]any {
	return d["props"].(map[string]any)["pageProps"].(map[string]any)["initialState"].(map[string]any)["program"].(map[string]any)["data"].(map[string]any)
}
func TestNetologyRejectsPlaceholdersPersonalPricesAndUnknownContracts(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value any
	}{{"price", 0}, {"initialPrice", 0}, {"price_type", "free"}, {"isFreeProgram", true}, {"isFake", true}, {"fromSandbox", true}, {"is_active", false}, {"program_family_url", "other-course"}, {"isDeferredPaymentProgram", true}, {"isAdditionalLessonApplied", true}, {"resource_packages", []any{map[string]any{"price": 100}}}, {"promoCodeInfo", map[string]any{"discount": 10}}, {"discount_finish_date", "2025-10-09"}} {
		t.Run(tc.key, func(t *testing.T) {
			data, c := netologyFixture(t, "java-developer")
			bad := netologyMutate(t, data, func(d map[string]any) { netologyProgramMap(d)[tc.key] = tc.value })
			if _, _, err := collectNetology(bad, c, netologyNow()); err == nil {
				t.Fatal("invalid contract accepted")
			}
		})
	}
}
func TestNetologyRequiresBoundOnePaymentCard(t *testing.T) {
	data, c := netologyFixture(t, "java-developer")
	for _, bad := range []string{"<html>challenge</html>", strings.ReplaceAll(string(data), "одним платежом", "ежемесячно"), strings.ReplaceAll(string(data), "131 700", "147 900"), strings.ReplaceAll(string(data), "sections[0].cards[0].title", "sections[0].cards[99].title"), strings.ReplaceAll(string(data), `href="https://netology.ru/programs/java-developer"`, `href="https://netology.ru/programs/another"`)} {
		if _, _, err := collectNetology([]byte(bad), c, netologyNow()); err == nil {
			t.Fatal("unbound payment accepted")
		}
	}
}
func TestNetologyClosedCohortsAndDateOnlyDiscountDeadline(t *testing.T) {
	data, c := netologyFixture(t, "go")
	_, o, err := collectNetology(data, c, time.Date(2026, 10, 9, 23, 0, 0, 0, time.FixedZone("Moscow", 10800)))
	if err != nil || o.ValidUntil == nil || o.ValidUntil.Format(time.RFC3339) != "2026-10-10T00:00:00+03:00" {
		t.Fatal(o, err)
	}
	if _, _, err = collectNetology(data, c, netologyNow().Add(24*time.Hour)); err == nil {
		t.Fatal("expired discount accepted")
	}
	data = netologyMutate(t, data, func(d map[string]any) {
		p := netologyProgramMap(d)
		p["discount_finish_date"] = "2030-01-01"
		p["isProgramStarted"] = true
	})
	_, o, err = collectNetology(data, c, netologyNow())
	if err != nil || o.Enrollment != "closed" {
		t.Fatal(o, err)
	}
}
func TestNetologyOfficialCatalogAndCandidates(t *testing.T) {
	f := Feed{ID: "netology", Adapter: "netology", URL: "https://netology.ru/development", Kind: "catalog"}
	if err := validateFeed(f); err != nil {
		t.Fatal(err)
	}
	for _, url := range []string{"https://netology.ru/programs/go", "https://netology.ru/programs/java-developer"} {
		if _, ok := candidateURL("netology", url, "netology"); !ok {
			t.Fatal(url)
		}
	}
	for _, url := range []string{"https://netology.ru/free-lessons/java", "https://netology.ru/programs/go/lesson", "https://evil.example/programs/go"} {
		if _, ok := candidateURL("netology", url, "netology"); ok {
			t.Fatal(url)
		}
	}
	f.URL = "https://netology.ru/profile"
	if validateFeed(f) == nil {
		t.Fatal("arbitrary catalog allowed")
	}
	config, err := LoadConfig("../../data/updater-sources.json", "../../data/real-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := config.ForWorker("pages")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range pages.Discovery {
		if f.Adapter == "netology" {
			found = true
		}
	}
	if !found {
		t.Fatal("Netology missing from pages role")
	}
}

func TestNetologyRejectsConflictingPlanCopies(t *testing.T) {
	data, c := netologyFixture(t, "java-developer")
	data = netologyMutate(t, data, func(d map[string]any) {
		content := d["props"].(map[string]any)["pageProps"].(map[string]any)["initialState"].(map[string]any)["landingContent"].(map[string]any)["content"].(map[string]any)
		for key, plan := range content {
			if !strings.HasPrefix(key, "plansNew_") {
				continue
			}
			raw, _ := json.Marshal(plan)
			var copy map[string]any
			json.Unmarshal(raw, &copy)
			copy["sections"].([]any)[0].(map[string]any)["cards"].([]any)[0].(map[string]any)["isOrderButtonHidden"] = true
			content["plansNew_conflicting"] = copy
			break
		}
	})
	for i := 0; i < 20; i++ {
		if _, _, err := collectNetology(data, c, netologyNow()); err == nil {
			t.Fatal("conflicting copies accepted")
		}
	}
}

func TestNetologyScheduledAdmissionEndsBeforeStartDate(t *testing.T) {
	data, c := netologyFixture(t, "go")
	data = netologyMutate(t, data, func(d map[string]any) { netologyProgramMap(d)["discount_finish_date"] = "2030-01-01" })
	start := time.Date(2026, 10, 19, 0, 0, 0, 0, time.FixedZone("Moscow", 10800))
	_, o, err := collectNetology(data, c, start.Add(-time.Second))
	if err != nil || o.Enrollment != "open" || o.ValidUntil == nil || !o.ValidUntil.Equal(start) {
		t.Fatal(o, err)
	}
	_, o, err = collectNetology(data, c, start)
	if err != nil || o.Enrollment != "closed" {
		t.Fatal(o, err)
	}
}
