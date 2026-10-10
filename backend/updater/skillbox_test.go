package updater

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func skillboxFixture(t *testing.T) ([]byte, Candidate) {
	t.Helper()
	b, e := os.ReadFile("testdata/skillbox-python.html")
	if e != nil {
		t.Fatal(e)
	}
	return b, Candidate{Adapter: "skillbox", ExternalID: "profession-python", URL: "https://skillbox.ru/course/profession-python", FeedID: "skillbox-sitemap"}
}
func TestSkillboxVerifiedFullCardPayments(t *testing.T) {
	b, c := skillboxFixture(t)
	r, o, e := collectSkillbox(b, c, time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Offers) != 3 || *r.Offers[0].Price != 13373500 || *r.Offers[1].Price != 18255100 || *r.Offers[2].Price != 21067200 || o.VerifiedProductID != 76 {
		t.Fatal(r, o)
	}
}
func TestSkillboxRejectsConflictsAndExpiredSale(t *testing.T) {
	b, c := skillboxFixture(t)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	for _, p := range [][2]string{{`Number("133735")`, `Number("133736")`}, {`Number("76")`, `Number("77")`}, {`2026-10-12 23:59:59`, `2020-10-12 23:59:59`}, {`currency_code: "RUB"`, `currency_code: "USD"`}, {`4 628`, `4 629`}, {`Картой всю сумму`, `Платёж в месяц`}, {`const priceInfo =`, `const otherInfo =`}} {
		bad := strings.ReplaceAll(string(b), p[0], p[1])
		if bad == string(b) {
			t.Fatal("ineffective mutation", p)
		}
		if _, _, e := collectSkillbox([]byte(bad), c, now); e == nil {
			t.Fatal("bad source accepted", p)
		}
	}
	if _, _, e := collectSkillbox(b, c, now.Add(4*24*time.Hour)); e == nil {
		t.Fatal("expired sale accepted")
	}
}
func TestSkillboxLiveVerification(t *testing.T) {
	if os.Getenv("SKILLBOX_LIVE_CHECK") != "1" {
		t.Skip("opt-in official reads")
	}
	_, c := skillboxFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	b, _, e := fetch(ctx, NewClient(), candidateEndpoint(c))
	if e != nil {
		t.Fatal(e)
	}
	r, _, e := collectSkillbox(b, c, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	t.Log(r.Title, len(r.Offers), *r.Offers[0].Price)
}
