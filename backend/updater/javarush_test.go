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

func jrFixture(t *testing.T) ([]byte, []byte, Candidate) {
	t.Helper()
	b, e := os.ReadFile("testdata/javarush-prices.html")
	if e != nil {
		t.Fatal(e)
	}
	p, e := os.ReadFile("testdata/javarush-monthly.json")
	if e != nil {
		t.Fatal(e)
	}
	return b, p, Candidate{Adapter: "javarush", ExternalID: "java-premium", URL: "https://javarush.com/prices", FeedID: "javarush-prices"}
}
func TestJavaRushOriginalRecurringCurrencyAndBudgetPresentation(t *testing.T) {
	b, p, c := jrFixture(t)
	r, o, e := collectJavaRush(b, p, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	offer := r.Offers[0]
	if len(r.Offers) != 1 || offer.Billing == nil || offer.Billing.AmountMinor != 3000 || offer.Billing.Currency != "USD" || offer.Billing.Interval != "month" || offer.Free || offer.Price != nil || !o.PriceUnknown || !strings.Contains(offer.Name, "30 USD в месяц") {
		t.Fatal(r, o)
	}
	max := int64(999999999)
	for _, f := range []catalog.Filter{{Max: &max}, {Budget: "free"}} {
		if rows := catalog.Search([]catalog.Course{r}, f, time.Now()); len(rows) != 0 {
			t.Fatal("subscription matched complete-price budget", rows)
		}
	}
	raw, _ := json.Marshal(r)
	if !strings.Contains(string(raw), `"amount_minor":3000`) || !strings.Contains(string(raw), `"price":null`) || strings.Contains(string(raw), "University —") {
		t.Fatal("API presentation lost billing semantics")
	}
}
func TestJavaRushRejectsUnboundCurrencyPeriodAndPlan(t *testing.T) {
	b, p, c := jrFixture(t)
	for _, pair := range [][2]string{{`"preferredCurrency": "USD"`, `"preferredCurrency": "EUR"`}, {`"usd": 30`, `"usd": 31`}, {`"MONTH"`, `"YEAR"`}, {`JR40_PREMIUM_JAVA_SELF`, `JR40_PREMIUM_JAVA_UNIV_PRO`}, {`"discountInfo": null`, `"discountInfo": {}`}} {
		bad := strings.ReplaceAll(string(p), pair[0], pair[1])
		if bad == string(p) {
			t.Fatal("ineffective mutation", pair)
		}
		if _, _, e := collectJavaRush(b, []byte(bad), c, time.Now()); e == nil {
			t.Fatal("invalid source accepted", pair)
		}
	}
	for _, pair := range [][2]string{{`$ в месяц`, `₽ в месяц`}, {`Java Premium`, `Java University`}, {`JR40_PREMIUM_JAVA_SELF`, `JR40_PREMIUM_JAVA_UNIV_PRO`}, {`>30<`, `>31<`}} {
		bad := strings.ReplaceAll(string(b), pair[0], pair[1])
		if bad == string(b) {
			t.Fatal("ineffective mutation", pair)
		}
		if _, _, e := collectJavaRush([]byte(bad), p, c, time.Now()); e == nil {
			t.Fatal("invalid card accepted", pair)
		}
	}
}
func TestJavaRushLiveVerification(t *testing.T) {
	if os.Getenv("JAVARUSH_LIVE_CHECK") != "1" {
		t.Skip("opt-in public anonymous GETs")
	}
	_, _, c := jrFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	b, d, e := fetch(ctx, NewClient(), c.URL)
	if e != nil {
		t.Fatal(e)
	}
	r, _, _, e := fetchJavaRush(ctx, NewClient(), b, d, c)
	if e != nil {
		t.Fatal(e)
	}
	t.Log(r.Title, r.Offers[0].Name)
}
