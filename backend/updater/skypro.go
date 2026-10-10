package updater

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"golang.org/x/net/html"
	"strconv"
	"strings"
	"time"
)

func collectSkypro(data []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	reject := func() (catalog.Course, Observation, error) {
		return catalog.Course{}, Observation{}, rejection("invalid_skypro_contract")
	}
	identity, ok := candidateURL("skypro", c.URL, c.FeedID)
	if c.Adapter != "skypro" || !ok || identity.ExternalID != c.ExternalID || identity.URL != c.URL {
		return reject()
	}
	doc, e := rsDocument(data, c.URL)
	if e != nil {
		return reject()
	}
	title := ""
	summary := ""
	titles := 0
	var pricingBlocks []*html.Node
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "h1" {
			title = nodeText(n)
			titles++
		}
		if n.Data == "meta" && nodeAttr(n, "name") == "description" {
			summary = nodeAttr(n, "content")
		}
		if n.Data == "div" && strings.Contains(" "+nodeAttr(n, "class")+" ", " t-rec ") && strings.Contains(nodeText(n), "Стоимость и варианты оплаты") {
			pricingBlocks = append(pricingBlocks, n)
		}
	})
	if titles != 1 || summary == "" || len(pricingBlocks) != 1 {
		return reject()
	}
	block := pricingBlocks[0]
	products := map[int64]bool{}
	malformed := false
	visitNodes(block, func(n *html.Node) {
		if nodeAttr(n, "class") != "tn-atom__inputs-data" {
			return
		}
		var fields []struct {
			Name  string `json:"li_nm"`
			Value string `json:"li_value"`
		}
		if json.Unmarshal([]byte(nodeAttr(n, "data-value")), &fields) != nil {
			malformed = true
			return
		}
		id := int64(0)
		kind := ""
		count := 0
		for _, f := range fields {
			if f.Name == "productId" {
				count++
				id, _ = strconv.ParseInt(f.Value, 10, 64)
			}
			if f.Name == "product_type" {
				kind = f.Value
			}
		}
		if count != 1 || id <= 0 || kind != "profession" {
			malformed = true
			return
		}
		products[id] = true
	})
	if malformed || len(products) != 1 {
		return reject()
	}
	product := int64(0)
	for id := range products {
		product = id
	}
	text := nodeText(block)
	// Only the target product's pricing/registration block can establish closure.
	// Similar diagnostic, webinar or other-course blocks outside it are ignored.
	closed := strings.Contains(text, "Набор временно закрыт") && strings.Contains(text, "Сейчас мы не принимаем новые заявки.")
	enrollment := "closed"
	if !closed {
		if strings.Contains(text, "Набор временно закрыт") || strings.Contains(text, "не принимаем новые заявки") || !strings.Contains(text, "Оставить заявку") || !strings.Contains(text, "ежемесячный платеж при рассрочке") {
			return catalog.Course{}, Observation{}, rejection("unverified_enrollment")
		}
		enrollment = "open"
	}
	lease := now.Add(26 * time.Hour)
	o := Observation{SkyproProductID: product, PriceUnknown: true, Enrollment: enrollment, Schedule: "unknown", ValidUntil: &lease}
	r, e := normalizedRecord(c, title, summary+" Полная стоимость обучения не подтверждена; месячный платёж рассрочки не является полной ценой.", "Skypro", o, now)
	if e != nil {
		return catalog.Course{}, Observation{}, e
	}
	r.Offers[0].Name = "Полная программа — стоимость уточняется"
	return r, o, nil
}
