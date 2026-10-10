package updater

import (
	"context"
	"devcourse-finder/catalog"
	"encoding/json"
	"fmt"
	"golang.org/x/net/html"
	"net/http"
	"strings"
	"time"
)

const jrMonthlyEndpoint = "https://javarush.com/api/1.0/rest/subscriptions/prices/all?duration=MONTH"

func collectJavaRush(data, prices []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	reject := func() (catalog.Course, Observation, error) {
		return catalog.Course{}, Observation{}, rejection("invalid_javarush_contract")
	}
	identity, ok := candidateURL("javarush", c.URL, c.FeedID)
	if c.Adapter != "javarush" || !ok || identity.URL != c.URL || identity.ExternalID != c.ExternalID {
		return reject()
	}
	doc, e := rsDocument(data, c.URL)
	if e != nil {
		return reject()
	}
	var cards []*html.Node
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "div" && nodeAttr(n, "class") == "payment-plan-card" {
			title := ""
			visitNodes(n, func(x *html.Node) {
				if nodeAttr(x, "class") == "payment-plan-card__title" {
					title = nodeText(x)
				}
			})
			if title == "Java Premium" {
				cards = append(cards, n)
			}
		}
	})
	if len(cards) != 1 {
		return reject()
	}
	card := cards[0]
	cost := ""
	period := ""
	costs := 0
	periods := 0
	buys := 0
	visitNodes(card, func(n *html.Node) {
		if nodeAttr(n, "class") == "payment-plan-card__cost" {
			cost = nodeText(n)
			costs++
		}
		if nodeAttr(n, "class") == "payment-plan-card__period" {
			period = nodeText(n)
			periods++
		}
		if n.Data == "a" && nodeAttr(n, "href") == "/prices/buy?key=JR40_PREMIUM_JAVA_SELF" && nodeText(n) == "Купить" && !academyHasAttr(n, "aria-disabled") && !academyHasAttr(n, "disabled") {
			buys++
		}
	})
	if costs != 1 || periods != 1 || period != "$ в месяц" || buys != 1 || strings.Contains(nodeAttr(card, "class"), "soon") || !strings.Contains(nodeText(card), "Для прохождения интерактивного Java‑курса без привязки к графику.") {
		return reject()
	}
	var all map[string]struct {
		Currency string                                `json:"preferredCurrency"`
		Prices   map[string]map[string]json.RawMessage `json:"prices"`
		Discount json.RawMessage                       `json:"discountInfo"`
	}
	if len(prices) > maxBody || decodeJSON(prices, &all) != nil {
		return reject()
	}
	p, ok := all["JR40_PREMIUM_JAVA_SELF"]
	if !ok || p.Currency != "USD" || string(p.Discount) != "null" {
		return reject()
	}
	amount, e := rubles(p.Prices["MONTH"]["usd"])
	if e != nil || amount <= 0 || amount > 1000000000 {
		return reject()
	}
	visible, e := rubles([]byte(cost))
	if e != nil || visible != amount {
		return reject()
	}
	lease := now.Add(26 * time.Hour)
	billing := &catalog.Billing{Kind: "subscription", AmountMinor: amount, Currency: "USD", Interval: "month"}
	o := Observation{PriceUnknown: true, Enrollment: "continuous", Schedule: "flexible", ValidUntil: &lease, Billing: billing}
	summary := "Самостоятельный интерактивный курс Java по подписке Premium, без привязки к расписанию. Регулярный платёж в исходной валюте; полная стоимость обучения зависит от длительности подписки и неизвестна. University и обучение с ментором — отдельные программы."
	r, e := normalizedRecord(c, "JavaRush: Java Premium — самостоятельное обучение", summary, "JavaRush", o, now)
	if e != nil {
		return catalog.Course{}, Observation{}, e
	}
	r.Offers[0].Billing = billing
	r.Offers[0].Name = fmt.Sprintf("Premium — %s USD в месяц", cost)
	return r, o, nil
}
func fetchJavaRush(ctx context.Context, client *http.Client, data []byte, digest string, c Candidate) (catalog.Course, Observation, string, error) {
	if _, e := rsDocument(data, c.URL); e != nil {
		return catalog.Course{}, Observation{}, digest, rejection("invalid_javarush_contract")
	}
	prices, pd, e := fetch(ctx, client, jrMonthlyEndpoint)
	if e != nil {
		return catalog.Course{}, Observation{}, digest, rejection("source_unavailable")
	}
	r, o, e := collectJavaRush(data, prices, c, time.Now().UTC())
	return r, o, fingerprint([]byte(digest + ":" + pd)), e
}
