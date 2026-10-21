package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"net/url"
	"regexp"
	"strings"
)

var hexletStudyLength = regexp.MustCompile(`^[0-9]{1,2} (месяц|месяца|месяцев)$`)
var hexletRecordID = regexp.MustCompile(`^(rec)?[0-9]+$`)

func aiHexletPaymentVariant(raw string, c Candidate) string {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Host != "ru.hexlet.io" || u.User != nil || u.Fragment != "" {
		return ""
	}
	q, e := url.ParseQuery(u.RawQuery)
	if e != nil {
		return ""
	}
	if u.Path == "/payments/new" && len(q) == 2 && len(q["program"]) == 1 && len(q["variant"]) == 1 && q.Get("program") == c.ExternalID {
		switch q.Get("variant") {
		case "standard":
			return "base"
		case "main":
			return "general"
		case "premium":
			return "premium"
		}
		return ""
	}
	if u.Path != "/programs/"+c.ExternalID+"/payment/new" {
		return ""
	}
	for key, v := range q {
		if (key != "price" && key != "payment_type") || len(v) != 1 {
			return ""
		}
	}
	if v, ok := q["payment_type"]; ok && v[0] != "self_payment" {
		return ""
	}
	price := q.Get("price")
	if len(q) == 0 {
		price = "general"
	}
	switch price {
	case "base", "general", "premium":
		return price
	}
	return ""
}
func aiHexletTariffDocuments(raw []byte, c Candidate, r catalog.Course, digest string) []AIDocument {
	doc := learningDocument(raw)
	out := []AIDocument{}
	for _, tier := range []struct{ key, name, hook, variant string }{{"standard", "Стандартный", "#zeropopup_standard", "base"}, {"extended", "Расширенный", "#zeropopup_extended", "general"}, {"premium", "Премиальный", "#zeropopup_premial", "premium"}} {
		offerID := r.ID + "-" + tier.key
		bound := false
		for _, o := range r.Offers {
			if o.ID == offerID && o.Name == tier.name+" — полная оплата" && aiSourceMatches(o.URL, c) {
				bound = true
			}
		}
		if !bound {
			continue
		}
		popups := learningNodes(doc, func(n *html.Node) bool {
			return classToken(n, "t-popup") && nodeAttr(n, "data-tooltip-hook") == tier.hook
		})
		if len(popups) != 1 {
			continue
		}
		rec := nodeAttr(popups[0], "data-popup-rec-ids")
		if !hexletRecordID.MatchString(rec) {
			continue
		}
		if !strings.HasPrefix(rec, "rec") {
			rec = "rec" + rec
		}
		roots := learningNodes(doc, func(n *html.Node) bool { return nodeAttr(n, "id") == rec })
		if len(roots) != 1 {
			continue
		}
		links := learningNodes(roots[0], func(n *html.Node) bool { return n.Data == "a" && aiHexletPaymentVariant(nodeAttr(n, "href"), c) != "" })
		if len(links) != 1 || aiHexletPaymentVariant(nodeAttr(links[0], "href"), c) != tier.variant {
			continue
		}
		match := func(n *html.Node) bool {
			if !classToken(n, "tn-molecule") || !strings.HasPrefix(aiNormalize(nodeText(n)), tier.name+" ") {
				return false
			}
			anchors := learningNodes(n, func(x *html.Node) bool { return x.Data == "a" && nodeAttr(x, "href") == tier.hook })
			lengths := learningNodes(n, func(x *html.Node) bool {
				return classToken(x, "tn-atom") && hexletStudyLength.MatchString(aiNormalize(nodeText(x)))
			})
			return len(anchors) == 1 && len(lengths) == 1
		}
		var cards []*html.Node
		visitNodes(doc, func(n *html.Node) {
			if match(n) {
				cards = append(cards, n)
			}
		})
		var selected []*html.Node
		for _, card := range cards {
			nested := false
			visitNodes(card, func(n *html.Node) {
				if n != card && match(n) {
					nested = true
				}
			})
			if !nested {
				selected = append(selected, card)
			}
		}
		if len(selected) != 1 {
			continue
		}
		texts := []string{}
		visitNodes(selected[0], func(n *html.Node) {
			if !classToken(n, "tn-atom") {
				return
			}
			text := aiNormalize(aiOfferText(n))
			if hexletStudyLength.MatchString(text) || strings.HasPrefix(text, "Нагрузка ") && strings.HasSuffix(text, "часов в неделю") || strings.Contains(text, "персональных консультаций с наставником") {
				texts = append(texts, text)
			}
		})
		if len(texts) > 0 {
			out = append(out, AIDocument{ID: "offer:" + offerID + ":public", Scope: "offer", OfferID: offerID, Partial: true, Source: c.URL, Text: tier.name + "\n" + strings.Join(texts, "\n"), Digest: digest})
		}
	}
	return out
}
func aiHexletPublicBundle(d []AIDocument, id string) bool {
	if len(d) < 1 || len(d) > 4 || !aiPartialPublicDocuments(d[:1]) {
		return false
	}
	for _, offer := range d[1:] {
		valid := false
		for _, key := range []string{"standard", "extended", "premium"} {
			if offer.OfferID == id+"-"+key {
				valid = true
			}
		}
		if !valid || offer.Scope != "offer" || !offer.Partial || offer.ID != "offer:"+offer.OfferID+":public" {
			return false
		}
	}
	return true
}
func aiHexletPublicTariffDigest(c Candidate, id, identity string, b *aiSavedBinding) string {
	return fingerprint([]byte("hexlet-public-tariffs-v2\x00" + aiHexletPublicDigest(c, id, identity, b)))
}
