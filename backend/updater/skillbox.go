package updater

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"golang.org/x/net/html"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var jsNumber = regexp.MustCompile(`Number\("([0-9]+)"\)`)
var jsBoolean = regexp.MustCompile(`Boolean\("[^"\\]*"\)`)
var jsKey = regexp.MustCompile(`([,{]\s*)([A-Za-z_][A-Za-z_0-9]*)\s*:`)
var jsSingle = regexp.MustCompile(`'([^'\\\r\n]*)'`)

func staticJSObject(data []byte, name string, target any) bool {
	pattern := regexp.MustCompile(`\bconst\s+` + regexp.QuoteMeta(name) + `\s*=\s*(\{[^{}]*\})\s*;`)
	matches := pattern.FindAllSubmatch(data, -1)
	if len(matches) != 1 {
		return false
	}
	b := string(matches[0][1])
	b = jsNumber.ReplaceAllString(b, "$1")
	b = jsBoolean.ReplaceAllString(b, "false")
	b = jsSingle.ReplaceAllStringFunc(b, func(v string) string { x, _ := json.Marshal(v[1 : len(v)-1]); return string(x) })
	b = jsKey.ReplaceAllString(b, `${1}"${2}":`)
	return decodeJSON([]byte(b), target) == nil
}
func collectSkillbox(data []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	reject := func() (catalog.Course, Observation, error) {
		return catalog.Course{}, Observation{}, rejection("invalid_skillbox_contract")
	}
	identity, ok := candidateURL("skillbox", c.URL, c.FeedID)
	if c.Adapter != "skillbox" || !ok || identity.ExternalID != c.ExternalID || identity.URL != c.URL {
		return reject()
	}
	doc, err := rsDocument(data, c.URL)
	if err != nil {
		return reject()
	}
	var landing struct {
		LandingID int64  `json:"landing_ID"`
		ProductID int64  `json:"nomenclature_ID"`
		Title     string `json:"title"`
		Language  string `json:"languageSlug"`
	}
	var payment struct {
		LandingID int64  `json:"landing_ID"`
		ProductID int64  `json:"nomenclature_ID"`
		Price     int64  `json:"price"`
		Currency  string `json:"currency_code"`
		URL       string `json:"landingUrl"`
		Service   string `json:"service"`
	}
	var prices struct {
		Discounted int64 `json:"discountedPrice"`
		Full       int64 `json:"fullPrice"`
		Monthly    int64 `json:"monthlyPayment"`
		Duration   int   `json:"installmentDuration"`
	}
	if !staticJSObject(data, "landingInfo", &landing) || !staticJSObject(data, "autopayment", &payment) || !staticJSObject(data, "priceInfo", &prices) || landing.LandingID <= 0 || landing.ProductID <= 0 || landing.Language != "ru" || landing.LandingID != payment.LandingID || landing.ProductID != payment.ProductID || payment.Currency != "RUB" || !matchingURL(payment.URL, c.URL) || payment.Service != "skillbox" || payment.Price <= 0 || prices.Full < prices.Discounted || prices.Discounted < payment.Price || prices.Monthly <= 0 || prices.Duration <= 0 {
		return reject()
	}
	titleMatches := 0
	summary := ""
	var cards []*html.Node
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "h1" && normalizeTitle(nodeText(n)) == normalizeTitle(landing.Title) {
			titleMatches++
		}
		if n.Data == "meta" && nodeAttr(n, "name") == "description" {
			summary = nodeAttr(n, "content")
		}
		if n.Data == "li" && nodeAttr(n, "data-tariff-data") != "" {
			cards = append(cards, n)
		}
	})
	if titleMatches != 1 || summary == "" || len(cards) == 0 || len(cards) > 3 {
		return reject()
	}
	lease := now.Add(26 * time.Hour)
	o := Observation{VerifiedProductID: landing.ProductID, Enrollment: "open", Schedule: "unknown", ValidUntil: &lease}
	defaults := 0
	seen := map[int64]bool{}
	for _, card := range cards {
		var t struct {
			ID      int64  `json:"nomenclature_id"`
			Default bool   `json:"isDefault"`
			Label   string `json:"label"`
			Title   string `json:"title"`
			Timer   string `json:"timer"`
			Price   struct {
				New        string `json:"new"`
				Currency   string `json:"newCurrency"`
				Additional string `json:"discountAmountWithCountryDiscount"`
			} `json:"price"`
			Payments []struct {
				Type        string `json:"methodType"`
				Title       string `json:"title"`
				Autopayment *struct {
					ID    int64 `json:"nomenclature_ID"`
					Price int64 `json:"price"`
				} `json:"autopayment"`
			} `json:"payments"`
		}
		if decodeJSON([]byte(nodeAttr(card, "data-tariff-data")), &t) != nil || t.ID <= 0 || seen[t.ID] || t.Title == "" || t.Price.Currency != "₽/мес" || !strings.Contains(nodeText(card), t.Title) {
			return reject()
		}
		switch t.Label {
		case "basic", "advanced", "vip":
		default:
			return reject()
		}
		zone := time.FixedZone("MSK", 3*3600)
		deadline, e := time.ParseInLocation("2006-01-02 15:04:05", t.Timer, zone)
		if e != nil || !deadline.After(now) {
			return reject()
		}
		if deadline.Before(lease) {
			lease = deadline
		}
		count := 0
		amount := int64(0)
		for _, p := range t.Payments {
			if p.Type != "autopayment" {
				continue
			}
			if p.Autopayment == nil || p.Autopayment.ID != t.ID || p.Autopayment.Price <= 0 || p.Autopayment.Price > 10000000 || p.Title != "Картой всю сумму" {
				return reject()
			}
			count++
			amount = p.Autopayment.Price
		}
		if count != 1 {
			return reject()
		}
		if t.Default {
			defaults++
			additional, e := strconv.ParseInt(strings.ReplaceAll(t.Price.Additional, " ", ""), 10, 64)
			monthly, e2 := strconv.ParseInt(strings.ReplaceAll(t.Price.New, " ", ""), 10, 64)
			if t.Label != "basic" || t.ID != landing.ProductID || amount != payment.Price || e != nil || e2 != nil || monthly != prices.Monthly || amount+additional != prices.Discounted {
				return reject()
			}
		}
		o.VerifiedTariffs = append(o.VerifiedTariffs, PurpleTariff{ID: t.ID, Category: t.Label, Name: t.Title + " — полная оплата картой", Price: amount * 100})
		seen[t.ID] = true
	}
	if defaults != 1 {
		return reject()
	}
	// Stable default-first ordering is necessary for the normalized observation.
	for i := range o.VerifiedTariffs {
		if o.VerifiedTariffs[i].Category == "basic" {
			o.VerifiedTariffs[0], o.VerifiedTariffs[i] = o.VerifiedTariffs[i], o.VerifiedTariffs[0]
		}
	}
	return verifiedTariffRecord(c, landing.Title, summary, o, now)
}
