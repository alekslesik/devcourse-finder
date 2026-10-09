package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

const practicumOrigin = "https://practicum.yandex.ru"

var productUUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

// Never follow an endpoint supplied by source HTML. The embedded map must agree
// exactly with the fixed, slug-bound endpoint before any API request is made.
func practicumPage(data []byte, c Candidate) error {
	if len(data) > maxBody || c.Adapter != "yandex" {
		return rejection("invalid_practicum_page")
	}
	identity, ok := candidateURL(c.Adapter, c.URL, c.FeedID)
	if !ok || identity.ExternalID != c.ExternalID || identity.URL != c.URL {
		return rejection("identity_mismatch")
	}
	tokenizer := html.NewTokenizer(bytes.NewReader(data))
	canonicals, configs := 0, 0
	canonicalOK, configOK, russian := false, false, false
	for {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			if tokenizer.Err() != io.EOF {
				return ErrSource
			}
			break
		}
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
			continue
		}
		t := tokenizer.Token()
		attrs := map[string]string{}
		for _, a := range t.Attr {
			attrs[a.Key] = a.Val
		}
		if t.Data == "html" && attrs["lang"] == "ru" {
			russian = true
		}
		if t.Data == "link" && attrs["rel"] == "canonical" {
			canonicals++
			canonicalOK = matchingURL(attrs["href"], c.URL)
		}
		if t.Data == "script" && attrs["id"] == "prices-config" {
			configs++
			if attrs["type"] != "application/json" || tokenizer.Next() != html.TextToken {
				return rejection("invalid_practicum_page")
			}
			var endpoints map[string]string
			if decodeJSON(tokenizer.Raw(), &endpoints) != nil {
				return rejection("invalid_practicum_page")
			}
			configOK = endpoints[c.ExternalID] == practicumPricesURL(c.ExternalID)
		}
	}
	if !russian || canonicals != 1 || !canonicalOK || configs != 1 || !configOK {
		return rejection("invalid_practicum_page")
	}
	// Product is used only to bind this landing to the course, never for SEO price
	// or availability. Its lowPrice can be an expired monthly payment.
	var nodes []map[string]any
	for _, m := range jsonLD.FindAllSubmatch(data, -1) {
		var value any
		if decodeJSON(m[1], &value) != nil {
			return rejection("invalid_practicum_page")
		}
		graphNodes(value, &nodes)
	}
	matched := 0
	for _, n := range nodes {
		if n["@type"] == "Product" && n["category"] == "Online Course" && n["sku"] == c.ExternalID && matchingURL(schemaString(n, "url"), c.URL) {
			matched++
		}
	}
	if matched != 1 {
		return rejection("identity_mismatch")
	}
	return nil
}
func practicumPricesURL(slug string) string {
	return practicumOrigin + "/api/v3/professions/prices/?slugs=" + slug
}
func practicumProfessionURL(slug string) string {
	return practicumOrigin + "/api/v2/professions-by-slugs/?slugs=" + slug
}
func practicumSquadsURL(slug string) string {
	return practicumOrigin + "/api/professions/" + slug + "/nearest_squads/"
}

type practicumProfession struct {
	ID             string          `json:"id"`
	Slug           string          `json:"slug"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Price          json.RawMessage `json:"price"`
	Currency       string          `json:"currency"`
	LandingPath    string          `json:"landing_path"`
	Type           string          `json:"type"`
	AbleToPurchase *bool           `json:"able_to_purchase"`
	HiddenType     json.RawMessage `json:"hidden_type"`
	DemandTest     *bool           `json:"is_demand_test"`
	Interactive    *bool           `json:"is_interactive_textbook"`
	Tariff         string          `json:"tariff"`
}
type practicumPrice struct {
	ProductID         string          `json:"profession_product_id"`
	Regular           *bool           `json:"profession_is_regular_charge"`
	Price             json.RawMessage `json:"profession_price"`
	Final             json.RawMessage `json:"profession_final_price"`
	Discount          *int64          `json:"total_absolute_discount"`
	Percent           *int64          `json:"percent_discount"`
	Deadline          *string         `json:"discount_deadline"`
	PromocodeDiscount *int64          `json:"absolute_promocode_discount"`
	Discount2035      *int64          `json:"discount_2035"`
	ChecklistDiscount *int64          `json:"checklist_discount"`
	PromoTariff       json.RawMessage `json:"promo_tariff"`
	Promocode         json.RawMessage `json:"promocode"`
	Certificates      json.RawMessage `json:"certificates"`
}
type practicumSquad struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Begin    string `json:"date_begin"`
	Deadline string `json:"payment_deadline"`
	Count    *int   `json:"subscriptions_num"`
	Limit    *int   `json:"subscriptions_limit"`
}

// Zone-less deadlines in the official Russian site are Moscow wall-clock times.
func practicumTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02T15:04:05", raw, time.FixedZone("Europe/Moscow", 3*60*60))
}
func emptyJSON(raw json.RawMessage, expected string) bool {
	var compact bytes.Buffer
	if json.Compact(&compact, raw) != nil {
		return false
	}
	return compact.String() == expected
}

func collectPracticum(page, profession, prices, squads []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	var empty catalog.Course
	var o Observation
	if len(profession) > maxBody || len(prices) > maxBody || len(squads) > maxBody {
		return empty, o, ErrSource
	}
	if err := practicumPage(page, c); err != nil {
		return empty, o, err
	}
	var programs []practicumProfession
	if decodeJSON(profession, &programs) != nil || len(programs) != 1 {
		return empty, o, rejection("invalid_practicum_payload")
	}
	p := programs[0]
	if p.Slug != c.ExternalID || !productUUID.MatchString(p.ID) || (p.LandingPath != "" && !matchingURL(practicumOrigin+p.LandingPath, c.URL)) {
		return empty, o, rejection("identity_mismatch")
	}
	if p.Currency != "RUB" || p.Type != "default" || p.AbleToPurchase == nil || !emptyJSON(p.HiddenType, "null") || p.DemandTest == nil || *p.DemandTest || p.Interactive == nil || *p.Interactive || (p.Tariff != "base" && p.Tariff != "plus") {
		return empty, o, rejection("invalid_practicum_payload")
	}
	var pricing map[string]map[string]practicumPrice
	if decodeJSON(prices, &pricing) != nil || len(pricing) != 1 {
		return empty, o, rejection("invalid_practicum_price")
	}
	byCurrency, exists := pricing[c.ExternalID]
	if !exists {
		return empty, o, rejection("identity_mismatch")
	}
	price, exists := byCurrency["RUB"]
	if !exists || !productUUID.MatchString(price.ProductID) || price.Regular == nil || *price.Regular {
		return empty, o, rejection("invalid_practicum_price")
	}
	base, err := rubles(price.Price)
	if err != nil || base <= 0 {
		return empty, o, rejection("invalid_practicum_price")
	}
	reference, err := rubles(p.Price)
	if err != nil || reference != base {
		return empty, o, rejection("invalid_practicum_price")
	}
	final, err := rubles(price.Final)
	if err != nil || final <= 0 || final > base || price.Discount == nil || *price.Discount < 0 || *price.Discount > 1000000000 || *price.Discount*100 != base-final || price.Percent == nil || *price.Percent < 0 || *price.Percent > 100 || (final == base && *price.Percent != 0) || (final < base && *price.Percent == 0) {
		return empty, o, rejection("invalid_practicum_price")
	}
	// API discounts are RUB, while the catalog stores kopeks. Allow at most
	// one ruble of integer percentage rounding when checking the percentage.
	expectedDiscount := base * *price.Percent / 100
	difference := (base - final) - expectedDiscount
	if difference < -100 || difference > 100 {
		return empty, o, rejection("invalid_practicum_price")
	}
	if price.PromocodeDiscount == nil || *price.PromocodeDiscount != 0 || price.Discount2035 == nil || *price.Discount2035 != 0 || price.ChecklistDiscount == nil || *price.ChecklistDiscount != 0 || !emptyJSON(price.PromoTariff, "{}") || !emptyJSON(price.Promocode, "{}") || !emptyJSON(price.Certificates, "[]") {
		return empty, o, rejection("invalid_practicum_price")
	}
	if final < base {
		if price.Deadline == nil {
			return empty, o, rejection("invalid_practicum_price")
		}
		deadline, err := practicumTime(*price.Deadline)
		if err != nil || !now.Before(deadline) {
			return empty, o, rejection("invalid_practicum_price")
		}
		o.ValidUntil = &deadline
	}
	var cohorts []practicumSquad
	if decodeJSON(squads, &cohorts) != nil || len(cohorts) > 1000 || strings.TrimSpace(string(squads)) == "null" {
		return empty, o, rejection("unverified_enrollment")
	}
	o.Enrollment = "closed"
	var lastDeadline time.Time
	for _, cohort := range cohorts {
		begin, beginErr := practicumTime(cohort.Begin)
		deadline, deadlineErr := practicumTime(cohort.Deadline)
		if cohort.ID <= 0 || !strings.HasPrefix(cohort.Name, c.ExternalID+"_cohort_") || beginErr != nil || begin.IsZero() || deadlineErr != nil || cohort.Count == nil || cohort.Limit == nil || *cohort.Count < 0 || *cohort.Limit <= 0 || *cohort.Count > *cohort.Limit {
			return empty, o, rejection("unverified_enrollment")
		}
		if *p.AbleToPurchase && now.Before(deadline) && *cohort.Count < *cohort.Limit {
			o.Enrollment = "open"
			if deadline.After(lastDeadline) {
				lastDeadline = deadline
			}
		}
	}
	if o.Enrollment == "open" && (o.ValidUntil == nil || lastDeadline.Before(*o.ValidUntil)) {
		o.ValidUntil = &lastDeadline
	}
	o.Price = &final
	o.Schedule = "scheduled"
	o.ProductID = price.ProductID
	o.ProfessionID = p.ID
	record, err := normalizedRecord(c, p.Name, p.Description, providerNames[c.Adapter], o, now)
	if err == nil && p.Tariff == "plus" {
		record.Offers[0].Name = "Расширенная программа"
	}
	return record, o, err
}

// Four independent official reads; all digests contribute to provenance. No
// authenticated, promotional, tracking, user or checkout endpoints are fetched.
func fetchPracticum(ctx context.Context, client *http.Client, page []byte, pageDigest string, c Candidate) (catalog.Course, Observation, string, error) {
	var empty catalog.Course
	if err := practicumPage(page, c); err != nil {
		return empty, Observation{}, pageDigest, err
	}
	bodies := make([][]byte, 3)
	digests := []string{pageDigest}
	for i, endpoint := range []string{practicumProfessionURL(c.ExternalID), practicumPricesURL(c.ExternalID), practicumSquadsURL(c.ExternalID)} {
		body, digest, err := fetch(ctx, client, endpoint)
		if err != nil {
			return empty, Observation{}, fingerprint([]byte(strings.Join(digests, ":"))), rejection("source_unavailable")
		}
		bodies[i] = body
		digests = append(digests, digest)
	}
	record, o, err := collectPracticum(page, bodies[0], bodies[1], bodies[2], c, time.Now().UTC())
	return record, o, fingerprint([]byte(strings.Join(digests, ":"))), err
}
