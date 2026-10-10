package updater

import (
	"context"
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

func collectSkillfactory(data, pricing []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	reject := func() (catalog.Course, Observation, error) {
		return catalog.Course{}, Observation{}, rejection("invalid_skillfactory_contract")
	}
	identity, ok := candidateURL("skillfactory", c.URL, c.FeedID)
	if c.Adapter != "skillfactory" || !ok || identity.URL != c.URL || identity.ExternalID != c.ExternalID {
		return reject()
	}
	doc, e := rsDocument(data, c.URL)
	if e != nil {
		return reject()
	}
	var rows []struct {
		Tag             string `json:"tag"`
		Title           string `json:"full_course_name"`
		URL             string `json:"url"`
		Start           string `json:"start_date"`
		Timer           string `json:"timer"`
		Currency        string `json:"currencySymbol"`
		Country         string `json:"siteCountryCode"`
		Basic           int64  `json:"price_discount_basic"`
		Consult         int64  `json:"price_discount_consult"`
		VIP             int64  `json:"price_discount_vip"`
		FullBasic       int64  `json:"price_full_basic"`
		FullConsult     int64  `json:"price_full_consult"`
		FullVIP         int64  `json:"price_full_vip"`
		DiscountBasic   int    `json:"discount_auto_basic"`
		DiscountConsult int    `json:"discount_auto_consult"`
		DiscountVIP     int    `json:"discount_auto_full"`
	}
	if len(pricing) > maxBody || decodeJSON(pricing, &rows) != nil || len(rows) != 1 {
		return reject()
	}
	p := rows[0]
	if !matchingURL(p.URL, c.URL) || p.Tag == "" || len(p.Tag) > 30 || p.Currency != "₽" || p.Country != "RU" {
		return reject()
	}
	titleCount := 0
	title := ""
	summary := ""
	script := false
	popups := map[string]int{}
	links := map[string]int{}
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "h1" {
			titleCount++
			title = nodeText(n)
		}
		if n.Data == "meta" && nodeAttr(n, "name") == "description" {
			summary = nodeAttr(n, "content")
		}
		if n.Data == "script" && nodeAttr(n, "src") == "https://tools.skillfactory.ru/js/price-automation.js" {
			script = true
		}
		if n.Data == "div" && nodeAttr(n, "class") == "t-popup" {
			popups[nodeAttr(n, "data-tooltip-hook")]++
		}
		if n.Data == "a" {
			links[nodeAttr(n, "href")]++
		}
	})
	if titleCount != 1 || summary == "" || !script || normalizeTitle(p.Title) != normalizeTitle("Профессия "+title) {
		return reject()
	}
	zone := time.FixedZone("MSK", 3*3600)
	deadline, e := time.ParseInLocation("2006-01-02 15:04:05", p.Timer, zone)
	if e != nil || !deadline.After(now) || deadline.Sub(now) > 45*24*time.Hour {
		return reject()
	}
	// A month-only start is usable only when the dated active campaign anchors a
	// single upcoming cohort within 45 days; never roll an old cohort forward.
	start, e := netologyStart(p.Start + " " + strconv.Itoa(deadline.Year()))
	if e != nil {
		return reject()
	}
	if start.Before(deadline) {
		return reject()
	}
	if start.Sub(deadline) > 45*24*time.Hour || !start.After(now) {
		return reject()
	}
	lease := now.Add(26 * time.Hour)
	if deadline.Before(lease) {
		lease = deadline
	}
	o := Observation{VerifiedProductID: int64Stable(p.Tag), Enrollment: "open", Schedule: "scheduled", ValidUntil: &lease}
	tiers := []struct {
		key, popup, name string
		amount, full     int64
		discount         int
	}{{"basic", "BASIC", "Базовый", p.Basic, p.FullBasic, p.DiscountBasic}, {"consult", "PERSONAL", "Персональный", p.Consult, p.FullConsult, p.DiscountConsult}, {"vip", "PERSONAL+", "Персональный+", p.VIP, p.FullVIP, p.DiscountVIP}}
	for i, t := range tiers {
		hook := "#popup:" + p.Tag + "-" + t.popup
		if popups[hook] != 1 || links[hook] == 0 || t.amount <= 0 || t.full < t.amount || t.full > 10000000 || t.discount < 0 || t.discount >= 100 || abs64(t.full*int64(100-t.discount)-t.amount*100) > 100 {
			return reject()
		}
		o.VerifiedTariffs = append(o.VerifiedTariffs, PurpleTariff{ID: int64(i + 1), Category: t.key, Name: t.name, Price: t.amount * 100})
	}
	return verifiedTariffRecord(c, title, summary, o, now)
}
func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
func int64Stable(s string) int64 {
	v, _ := strconv.ParseInt(fingerprint([]byte(s))[:15], 16, 64)
	return v + 1
}
func fetchSkillfactory(ctx context.Context, client *http.Client, data []byte, digest string, c Candidate) (catalog.Course, Observation, string, error) {
	// The catalog feed and canonical document bind the course before an official
	// fixed-path GET. Lead/contact/payment POST endpoints are never called.
	if _, e := rsDocument(data, c.URL); e != nil {
		return catalog.Course{}, Observation{}, digest, rejection("invalid_skillfactory_contract")
	}
	endpoint := "https://tools.skillfactory.ru/api/get-info-by-url?url=" + url.QueryEscape(c.URL+"/")
	pricing, pd, e := fetch(ctx, client, endpoint)
	if e != nil {
		return catalog.Course{}, Observation{}, digest, rejection("source_unavailable")
	}
	r, o, e := collectSkillfactory(data, pricing, c, time.Now().UTC())
	return r, o, fingerprint([]byte(digest + ":" + pd)), e
}
