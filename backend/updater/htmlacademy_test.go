package updater

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func academyEnvelope(payload []byte) []byte {
	token := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + base64.RawURLEncoding.EncodeToString(payload) + ".fixture-signature"
	b, _ := json.Marshal(map[string]string{"token": token})
	return b
}
func academyFixture(t *testing.T, slug string) ([]byte, []byte, Candidate) {
	t.Helper()
	page, err := os.ReadFile("testdata/htmlacademy-" + slug + ".html")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := os.ReadFile("testdata/htmlacademy-" + slug + "-payment.json")
	if err != nil {
		t.Fatal(err)
	}
	return page, payload, Candidate{Adapter: "htmlacademy", ExternalID: "intensive-" + slug, URL: "https://htmlacademy.ru/intensive/" + slug, FeedID: "htmlacademy-sitemap"}
}
func TestHTMLAcademyRecordedRecurringCourses(t *testing.T) {
	for _, slug := range []string{"javascript", "react"} {
		b, p, c := academyFixture(t, slug)
		r, o, err := collectHTMLAcademy(b, academyEnvelope(p), c, netologyNow())
		if err != nil {
			t.Fatal(slug, err)
		}
		if r.Language != "javascript" || !o.PriceUnknown || o.Price != nil || o.Enrollment != "continuous" || o.Schedule != "flexible" || r.Offers[0].Price != nil || r.Offers[0].Free || r.Offers[0].PriceKind != "unknown" || r.Offers[0].Mentor || r.Offers[0].Review {
			t.Fatal(r, o)
		}
		if o.ValidUntil == nil || !o.ValidUntil.Equal(netologyNow().Add(26*time.Hour)) {
			t.Fatal(o)
		}
	}
}
func TestHTMLAcademyRejectsPaymentMismatchesAndInactiveProducts(t *testing.T) {
	b, p, c := academyFixture(t, "javascript")
	for _, pair := range [][2]string{
		{`"productSubtype": "javascript-individual"`, `"productSubtype": "react-individual"`},
		{`"productType": "intensiveext"`, `"productType": "profession"`},
		{`"currency": "RUB"`, `"currency": "USD"`},
		{`"amount": 12000`, `"amount": 11000`},
		{`"period": "monthly"`, `"period": "yearly"`},
		{`"recurrent": true`, `"recurrent": false`},
		{`"places": null`, `"places": 0`},
		{`"dates": []`, `"dates": ["2020-01-01"]`},
		{`"dates": []`, `"removedDates": []`},
		{`"places": null`, `"removedPlaces": null`},
		{`"id": "default"`, `"id": "lite"`},
		{`"ru.card.sbp"`, `"corporate"`},
		{`Курс «JavaScript. Профессиональная разработка веб-интерфейсов»`, `Курс «Другой JavaScript»`},
	} {
		bad := strings.ReplaceAll(string(p), pair[0], pair[1])
		if pair[0] == `"ru.card.sbp"` {
			bad = strings.ReplaceAll(bad, "ru.card.gazprombank", "corporate")
		}
		if bad == string(p) {
			t.Fatal("test mutation ineffective", pair)
		}
		if _, _, err := collectHTMLAcademy(b, academyEnvelope([]byte(bad)), c, netologyNow()); err == nil {
			t.Fatal("invalid payment accepted", pair)
		}
	}
	for _, bad := range [][]byte{nil, []byte(`{"token":"garbage"}`), academyEnvelope([]byte(`{"offers":[]}`)), academyEnvelope([]byte(`{}`))} {
		if _, _, err := collectHTMLAcademy(b, bad, c, netologyNow()); err == nil {
			t.Fatal("invalid payment accepted")
		}
	}
}
func TestHTMLAcademyRejectsArchivedOrUnboundPages(t *testing.T) {
	b, p, c := academyFixture(t, "javascript")
	for _, pair := range [][2]string{
		{`lang="ru"`, `lang="en"`},
		{`https://htmlacademy.ru/intensive/javascript`, `https://htmlacademy.ru/intensive/react`},
		{`data-pay-tariff="individual"`, `data-pay-tariff="lite"`},
		{`data-pay-tariff="individual"`, `disabled data-pay-tariff="individual"`},
		{`/api/payment-data/intensive/javascript/individual`, `https://evil.example/api`},
		{`Помесячная оплата, без банковских рассрочек и кредитов`, `Архивный курс`},
		{`12 000 руб. в месяц`, `Бесплатный пробный урок`},
	} {
		bad := strings.ReplaceAll(string(b), pair[0], pair[1])
		if bad == string(b) {
			t.Fatal("test mutation ineffective", pair)
		}
		if _, _, err := collectHTMLAcademy([]byte(bad), academyEnvelope(p), c, netologyNow()); err == nil {
			t.Fatal("invalid page accepted", pair)
		}
	}
	c.URL = "https://htmlacademy.ru/profession/react"
	c.ExternalID = "profession-react"
	if _, _, err := collectHTMLAcademy(b, academyEnvelope(p), c, netologyNow()); rejectionCode(err, "") != "unsupported_htmlacademy_format" {
		t.Fatal(err)
	}
}
func TestHTMLAcademyDiscoveryBoundaries(t *testing.T) {
	for _, raw := range []string{"https://htmlacademy.ru/intensive/javascript/reviews", "https://htmlacademy.ru/profession/react/individual", "https://htmlacademy.ru/courses/basic", "https://htmlacademy.ru/payment", "https://evil.example/intensive/react"} {
		if _, ok := candidateURL("htmlacademy", raw, ""); ok {
			t.Fatal(raw)
		}
	}
	f := Feed{ID: "htmlacademy-sitemap", Adapter: "htmlacademy", URL: "https://htmlacademy.ru/sitemap.xml"}
	children := childSitemaps(f, []string{"https://htmlacademy.ru/sitemap/sitemap_default.xml", "https://htmlacademy.ru/sitemap/sitemap_courses.xml", "https://evil.example/sitemap.xml"})
	if len(children) != 1 || children[0] != "https://htmlacademy.ru/sitemap/sitemap_default.xml" {
		t.Fatal(children)
	}
}
func TestHTMLAcademyLiveVerification(t *testing.T) {
	if os.Getenv("HTMLACADEMY_LIVE_CHECK") != "1" {
		t.Skip("opt-in official anonymous reads")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
	defer cancel()
	for _, slug := range []string{"javascript", "react"} {
		_, _, c := academyFixture(t, slug)
		page, digest, err := fetch(ctx, NewClient(), c.URL)
		if err != nil {
			t.Fatal(err)
		}
		r, o, _, err := fetchHTMLAcademy(ctx, NewClient(), page, digest, c)
		if err != nil {
			t.Fatal(slug, err)
		}
		t.Log(r.Title, o.Enrollment, r.Offers[0].PriceKind)
	}
}
