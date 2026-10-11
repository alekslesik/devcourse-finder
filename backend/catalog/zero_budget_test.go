package catalog

import (
	"net/url"
	"testing"
	"time"
)

func TestZeroMinimumDoesNotRequireKnownPrice(t *testing.T) {
	now := time.Now().UTC()
	c := fixture(now)
	c.Offers = append([]Offer(nil), c.Offers[:1]...)
	c.Offers[0].Price = nil
	c.Offers[0].PriceKind = "unknown"
	for _, filter := range []Filter{{}, {Min: ptr(int64(0))}} {
		rows := Search([]Course{c}, filter, now)
		if len(rows) != 1 {
			t.Fatal("unrestricted price lost unknown course", filter)
		}
		for _, reason := range rows[0].Reasons {
			if reason == "В пределах бюджета" {
				t.Fatal("unknown price claimed within budget")
			}
		}
	}
	for _, filter := range []Filter{{Min: ptr(int64(1))}, {Min: ptr(int64(0)), Max: ptr(int64(999999999))}} {
		if len(Search([]Course{c}, filter, now)) != 0 {
			t.Fatal("bounded budget accepted unknown", filter)
		}
	}
	c.Offers[0].Billing = &Billing{Kind: "subscription", AmountMinor: 1000, Currency: "RUB", Interval: "month"}
	if len(Search([]Course{c}, Filter{Min: ptr(int64(0))}, now)) != 1 || len(Search([]Course{c}, Filter{Max: ptr(int64(999999999))}, now)) != 0 {
		t.Fatal("subscription treated as known course total")
	}
	parsed, err := Parse(url.Values{"min": {"0"}})
	if err != nil || len(Search([]Course{c}, parsed, now)) != 1 {
		t.Fatal(parsed, err)
	}
}
