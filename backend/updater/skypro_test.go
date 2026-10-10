package updater

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func skyFixture(t *testing.T, slug string) ([]byte, Candidate) {
	t.Helper()
	b, e := os.ReadFile("testdata/skypro-" + slug + ".html")
	if e != nil {
		t.Fatal(e)
	}
	id := map[string]string{"python": "python-web-course", "java": "java-developer"}[slug]
	return b, Candidate{Adapter: "skypro", ExternalID: id, URL: "https://sky.pro/course/programming/" + id, FeedID: "skypro-catalog"}
}
func TestSkyproRecordedClosedEnrollmentAndUnknownPrice(t *testing.T) {
	for _, slug := range []string{"python", "java"} {
		b, c := skyFixture(t, slug)
		r, o, e := collectSkypro(b, c, time.Now().UTC())
		if e != nil {
			t.Fatal(slug, e)
		}
		if o.Enrollment != "closed" || !o.PriceUnknown || o.Price != nil || r.Offers[0].Price != nil || r.Offers[0].Free || o.SkyproProductID <= 0 {
			t.Fatal(r, o)
		}
	}
}
func TestSkyproScopesClosureToTargetProduct(t *testing.T) {
	b, c := skyFixture(t, "python")
	// Remove only closure copy inside target pricing block; leave unrelated closed
	// diagnostics in the recorded fixture, and retain the enabled application CTA.
	s := string(b)
	start := strings.Index(s, `<div class="r t-rec">`)
	end := strings.Index(s[start:], `</p>`) + start
	scoped := s[start:end]
	scoped = strings.ReplaceAll(scoped, "Набор временно закрыт", "")
	scoped = strings.ReplaceAll(scoped, "Сейчас мы не принимаем новые заявки.", "")
	s = s[:start] + scoped + s[end:]
	r, o, e := collectSkypro([]byte(s), c, time.Now().UTC())
	if e != nil || o.Enrollment != "open" || r.Offers[0].Price != nil {
		t.Fatal(r, o, e)
	}
}
func TestSkyproRejectsUnboundAndAmbiguousPricing(t *testing.T) {
	b, c := skyFixture(t, "python")
	for _, p := range [][2]string{{`Стоимость и варианты оплаты`, `Зарплаты выпускников`}, {`&quot;profession&quot;`, `&quot;webinar&quot;`}, {`Набор временно закрыт`, `Старт скоро`}, {`python-web-course`, `other-course`}} {
		bad := strings.ReplaceAll(string(b), p[0], p[1])
		if bad == string(b) {
			t.Fatal("ineffective mutation", p)
		}
		if _, _, e := collectSkypro([]byte(bad), c, time.Now().UTC()); e == nil {
			t.Fatal("invalid source accepted", p)
		}
	}
}
func TestSkyproLiveVerification(t *testing.T) {
	if os.Getenv("SKYPRO_LIVE_CHECK") != "1" {
		t.Skip("opt-in official GETs only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	for _, slug := range []string{"python", "java"} {
		_, c := skyFixture(t, slug)
		b, _, e := fetch(ctx, NewClient(), c.URL)
		if e != nil {
			t.Fatal(e)
		}
		r, o, e := collectSkypro(b, c, time.Now().UTC())
		if e != nil {
			t.Fatal(slug, e)
		}
		t.Log(r.Title, o.Enrollment, r.Offers[0].PriceKind)
	}
}
