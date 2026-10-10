package updater

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func sfFixture(t *testing.T) ([]byte, []byte, Candidate) {
	t.Helper()
	b, e := os.ReadFile("testdata/skillfactory-python.html")
	if e != nil {
		t.Fatal(e)
	}
	p, e := os.ReadFile("testdata/skillfactory-python-pricing.json")
	if e != nil {
		t.Fatal(e)
	}
	return b, p, Candidate{Adapter: "skillfactory", ExternalID: "python-developer", URL: "https://skillfactory.ru/python-developer", FeedID: "skillfactory-catalog"}
}
func TestSkillfactoryVerifiedCourseTariffs(t *testing.T) {
	b, p, c := sfFixture(t)
	r, o, e := collectSkillfactory(b, p, c, time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	if e != nil {
		t.Fatal(e)
	}
	if len(r.Offers) != 3 || *r.Offers[0].Price != 15404400 || *r.Offers[1].Price != 19764000 || *r.Offers[2].Price != 25239600 || o.Enrollment != "open" || o.Schedule != "scheduled" {
		t.Fatal(r, o)
	}
}
func TestSkillfactoryRejectsStaleAndUnboundPricing(t *testing.T) {
	b, p, c := sfFixture(t)
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	for _, pair := range [][2]string{{`154044`, `4279`}, {`2026-10-13`, `2020-10-13`}, {`1 ноября`, `1 января`}, {`python-developer`, `java-developer`}, {`"siteCountryCode": "RU"`, `"siteCountryCode": "US"`}, {`"tag": "PDEV"`, `"tag": "OTHER"`}} {
		bad := strings.ReplaceAll(string(p), pair[0], pair[1])
		if bad == string(p) {
			t.Fatal("ineffective mutation", pair)
		}
		if _, _, e := collectSkillfactory(b, []byte(bad), c, now); e == nil {
			t.Fatal("invalid pricing accepted", pair)
		}
	}
	for _, pair := range [][2]string{{`PDEV-BASIC`, `OTHER-BASIC`}, {`price-automation.js`, `other.js`}} {
		bad := strings.ReplaceAll(string(b), pair[0], pair[1])
		if bad == string(b) {
			t.Fatal("ineffective mutation", pair)
		}
		if _, _, e := collectSkillfactory([]byte(bad), p, c, now); e == nil {
			t.Fatal("changed page accepted", pair)
		}
	}
}
func TestSkillfactoryLiveVerification(t *testing.T) {
	if os.Getenv("SKILLFACTORY_LIVE_CHECK") != "1" {
		t.Skip("opt-in official reads")
	}
	_, _, c := sfFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	b, d, e := fetch(ctx, NewClient(), c.URL)
	if e != nil {
		t.Fatal(e)
	}
	r, _, _, e := fetchSkillfactory(ctx, NewClient(), b, d, c)
	if e != nil {
		t.Fatal(e)
	}
	t.Log(r.Title, len(r.Offers), *r.Offers[0].Price)
}
