package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"
)

var testSource = Source{CourseID: "stepik-golang", Adapter: "stepik", RemoteID: 54403, ExpectedTitle: "Программирование на Golang"}

func validStepik() map[string]any {
	return map[string]any{
		"id": 54403, "title": "Программирование на Golang", "is_public": true, "is_active": true, "is_enabled": true, "is_archived": false, "is_censored": false, "is_paid": false, "is_self_paced": true, "price": nil, "currency_code": nil,
		"actions": map[string]any{"can_be_enrolled": map[string]any{"enabled": true}},
	}
}
func stepikJSON(course map[string]any) []byte {
	raw, _ := json.Marshal(map[string]any{"courses": []any{course}})
	return raw
}
func TestStepikRejectsIncompleteAndWrongIdentity(t *testing.T) {
	for _, field := range []string{"id", "title", "is_public", "is_active", "is_enabled", "is_archived", "is_censored", "is_paid"} {
		t.Run(field, func(t *testing.T) {
			c := validStepik()
			delete(c, field)
			if _, err := parseStepik(stepikJSON(c), testSource, time.Now()); err == nil {
				t.Fatal("incomplete source accepted")
			}
		})
	}
	for _, change := range []struct {
		field string
		value any
	}{{"id", 999}, {"title", "Other course"}, {"is_public", false}, {"is_censored", true}, {"is_active", false}, {"price", "100.00"}} {
		c := validStepik()
		c[change.field] = change.value
		if _, err := parseStepik(stepikJSON(c), testSource, time.Now()); err == nil {
			t.Fatalf("accepted contradictory %s", change.field)
		}
	}
}
func TestStepikExplicitPricesAndAvailability(t *testing.T) {
	now := time.Now().UTC()
	c := validStepik()
	o, err := parseStepik(stepikJSON(c), testSource, now)
	if err != nil || o.Price == nil || *o.Price != 0 || o.Enrollment != "continuous" || o.Schedule != "flexible" {
		t.Fatalf("free course not verified: %#v %v", o, err)
	}
	c["is_paid"] = true
	c["price"] = "119000.50"
	c["currency_code"] = "RUB"
	o, err = parseStepik(stepikJSON(c), testSource, now)
	if err != nil || o.Price == nil || *o.Price != 11900050 {
		t.Fatalf("kopeck conversion: %#v %v", o, err)
	}
	for _, currency := range []string{"USD", "", "rub"} {
		c["currency_code"] = currency
		o, err = parseStepik(stepikJSON(c), testSource, now)
		if err != nil || o.Price != nil || !o.PriceUnknown {
			t.Fatalf("unverified currency accepted: %#v %v", o, err)
		}
	}
	c = validStepik()
	delete(c, "actions")
	o, err = parseStepik(stepikJSON(c), testSource, now)
	if err != nil || o.Enrollment != "" {
		t.Fatal("missing actions fabricated open enrollment")
	}
	c["actions"] = map[string]any{"can_be_enrolled": map[string]any{"enabled": false}}
	o, err = parseStepik(stepikJSON(c), testSource, now)
	if err != nil || o.Enrollment != "" {
		t.Fatal("account restriction fabricated closed enrollment")
	}
	c["enrollment_end_date"] = now.Add(-time.Hour)
	o, err = parseStepik(stepikJSON(c), testSource, now)
	if err != nil || o.Enrollment != "closed" {
		t.Fatal("explicit closed enrollment not observed")
	}
	c = validStepik()
	c["is_archived"] = true
	o, err = parseStepik(stepikJSON(c), testSource, now)
	if err != nil || o.Enrollment != "closed" || o.Price != nil {
		t.Fatal("archival refreshed price")
	}
}
func TestPriceParserRejectsMonthlyFormattedAndExtremeValues(t *testing.T) {
	for _, raw := range []string{`"от 5 000 ₽/месяц"`, `"1,200"`, `"-1"`, `1e3`, `null`, `99999999999999999999`, `0.001`, `"01"`} {
		if _, err := rubles([]byte(raw)); err == nil {
			t.Fatalf("ambiguous price accepted: %s", raw)
		}
	}
}
func TestSchemaRequiresFullCourseEvidence(t *testing.T) {
	canonical := "https://code-basics.com/ru/languages/go"
	node := map[string]any{"@type": "Course", "url": canonical, "name": "Golang", "isAccessibleForFree": true, "offers": map[string]any{"@type": "Offer", "price": "0.00", "priceCurrency": "RUB", "availability": "https://schema.org/InStock"}}
	html := func() []byte {
		b, _ := json.Marshal(node)
		return []byte(`<html><script type="application/ld+json">` + string(b) + `</script></html>`)
	}
	if o, err := parseSchemaCourse(html(), canonical); err != nil || o.Price == nil || *o.Price != 0 || o.Enrollment != "continuous" {
		t.Fatal("verified schema rejected")
	}
	for _, field := range []string{"url", "isAccessibleForFree", "offers", "name"} {
		saved := node[field]
		delete(node, field)
		if _, err := parseSchemaCourse(html(), canonical); err == nil {
			t.Fatalf("missing %s accepted", field)
		}
		node[field] = saved
	}
	offer := node["offers"].(map[string]any)
	offer["priceSpecification"] = map[string]any{"billingDuration": "P1M"}
	if _, err := parseSchemaCourse(html(), canonical); err == nil {
		t.Fatal("installment accepted")
	}
	if _, err := parseSchemaCourse([]byte("<html>Access denied</html>"), canonical); err == nil {
		t.Fatal("challenge published")
	}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestFetcherRejectsFailuresOversizeAndUnapprovedURLs(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		status int
		body   string
	}{{"missing", 404, ""}, {"redirect", 302, ""}, {"oversize", 200, strings.Repeat("x", maxBody+1)}} {
		t.Run(scenario.name, func(t *testing.T) {
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: scenario.status, Body: io.NopCloser(strings.NewReader(scenario.body)), Header: http.Header{}, Request: r}, nil
			})}
			if _, _, err := fetch(context.Background(), client, "https://stepik.org/api/courses/54403"); err == nil {
				t.Fatal("unsafe response accepted")
			}
		})
	}
	for _, url := range []string{"http://stepik.org/api/courses/54403", "https://127.0.0.1/", "https://stepik.org.evil.test/", "https://secret@stepik.org/"} {
		if _, _, err := fetch(context.Background(), nil, url); err == nil {
			t.Fatalf("unsafe URL accepted %s", url)
		}
	}
	for _, raw := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::1", "fc00::1", "0.0.0.0"} {
		ip := netip.MustParseAddr(raw)
		if publicIP(ip.AsSlice()) {
			t.Fatalf("private destination accepted: %s", raw)
		}
	}
}
func TestNextRunMoscowBoundaries(t *testing.T) {
	for _, tc := range []struct{ now, want string }{
		{"2026-10-08T05:59:59Z", "2026-10-08T06:00:00Z"},
		{"2026-10-08T06:00:00Z", "2026-10-08T18:00:00Z"},
		{"2026-10-08T18:00:00Z", "2026-10-09T06:00:00Z"},
		{"2026-12-31T19:00:00Z", "2027-01-01T06:00:00Z"},
	} {
		now, _ := time.Parse(time.RFC3339, tc.now)
		want, _ := time.Parse(time.RFC3339, tc.want)
		if got := NextRun(now); !got.Equal(want) {
			t.Fatalf("%s -> %s, want %s", tc.now, got, want)
		}
	}
}

func TestCheckedInConfigurationAndTemplates(t *testing.T) {
	config, err := LoadConfig("../../data/updater-sources.json", "../../data/real-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Sources) != 2 {
		t.Fatal("unexpected initial source coverage")
	}
	config.Sources[0].RemoteID = 58852
	if config.Validate() == nil {
		t.Fatal("mismatched remote identity accepted")
	}
}

// Fixtures retain only public contract fields from real anonymous API responses,
// including null prices and account-specific enrollment actions.
func TestRecordedOfficialResponses(t *testing.T) {
	cases := []struct {
		file   string
		source Source
	}{
		{"testdata/stepik-go.json", testSource},
		{"testdata/stepik-python.json", Source{RemoteID: 58852, ExpectedTitle: "«Поколение Python»: курс для начинающих"}},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			o, err := parseStepik(raw, tc.source, time.Now())
			if err != nil || o.Price == nil || *o.Price != 0 || o.Enrollment != "continuous" {
				t.Fatalf("official contract rejected: %#v %v", o, err)
			}
		})
	}
}

func TestRedirectCannotReachPrivateOrForeignHost(t *testing.T) {
	client := NewClient()
	calls := 0
	client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{"http://169.254.169.254/latest/meta-data/"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
	})
	if _, _, err := fetch(context.Background(), client, "https://stepik.org/api/courses/54403"); err == nil || calls != 1 {
		t.Fatalf("followed redirect: calls=%d err=%v", calls, err)
	}
}
