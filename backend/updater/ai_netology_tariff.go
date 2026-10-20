package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strings"
)

// A public selected-program card can attest its explicitly labelled study
// length, not a checkout ID, installment period or support for another plan.
func aiNetologyPublicDuration(raw []byte, c Candidate, current catalog.Course, b aiNetologyBinding, digest string) *AIDocument {
	var offer *catalog.Offer
	for i := range current.Offers {
		o := &current.Offers[i]
		if o.ID == b.OfferID && aiSourceMatches(o.URL, c) && normalizeTitle(o.Name) == normalizeTitle(current.Title) {
			offer = o
		}
	}
	if offer == nil {
		return nil
	}
	doc := learningDocument(raw)
	regs := learningNodes(doc, func(n *html.Node) bool {
		return strings.HasPrefix(nodeAttr(n, "data-contentkey"), "programRegistration")
	})
	if len(regs) != 1 {
		return nil
	}
	titles := learningNodes(regs[0], func(n *html.Node) bool { return n.Data == "p" && classToken(n, "styles_title__RPcv7") })
	if len(titles) != 1 || normalizeTitle(nodeText(titles[0])) != normalizeTitle(offer.Name) {
		return nil
	}
	// This is the rendered selected-program price card, not the surrounding lead
	// form or the sibling mobile installment summary. Only its study-length label
	// enters the offer document; all price/billing/contact text stays excluded.
	for root := titles[0].Parent; root != nil && root != regs[0]; root = root.Parent {
		if !classToken(root, "styles_root__ksXoR") {
			continue
		}
		labels := learningNodes(root, func(n *html.Node) bool { return n.Data == "p" && classToken(n, "styles_header__WXLp2") })
		if len(labels) != 1 {
			return nil
		}
		text := aiNormalize(nodeText(labels[0]))
		if !strings.HasSuffix(text, "месяцев обучения") && !strings.HasSuffix(text, "месяца обучения") && !strings.HasSuffix(text, "месяц обучения") {
			return nil
		}
		// Reject a second named program or a duplicate duration shell.
		if len(learningNodes(root, func(n *html.Node) bool { return n.Data == "p" && classToken(n, "styles_title__RPcv7") })) != 1 {
			return nil
		}
		return &AIDocument{ID: "offer:" + offer.ID + ":duration", Scope: "offer", OfferID: offer.ID, Partial: true, Source: c.URL, Text: text, Digest: digest}
	}
	return nil
}
func aiNetologyPublicBundle(docs []AIDocument, b aiNetologyBinding) bool {
	if len(docs) < 1 || len(docs) > 2 || docs[0].ID != "course-public" || docs[0].Scope != "course" || !docs[0].Partial || docs[0].OfferID != "" {
		return false
	}
	return len(docs) == 1 || docs[1].ID == "offer:"+b.OfferID+":duration" && docs[1].Scope == "offer" && docs[1].OfferID == b.OfferID && docs[1].Partial
}
func aiNetologyPublicTariffDigest(id string, b aiNetologyBinding) string {
	return fingerprint([]byte("netology-public-program-duration-v2\x00" + aiNetologyBindingDigest(id, b)))
}
