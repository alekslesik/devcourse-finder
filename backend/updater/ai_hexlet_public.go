package updater

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"golang.org/x/net/html"
	"net/url"
	"strings"
)

// This is a separate course-only identity contract for public landings without
// Course/Product JSON-LD. It never manufactures a native price/tariff mapping.
func aiHexletPublic(raw []byte, c Candidate, current catalog.Course, b *aiSavedBinding, digest string) ([]AIDocument, error) {
	if c.Adapter != "hexlet" || !aiSourceMatches(current.Source, c) {
		return nil, ErrAI
	}
	doc, err := rsDocument(raw, c.URL)
	if err != nil {
		return nil, ErrAI
	}
	title, description := "", ""
	heads := 0
	invalid := false
	variants := map[string]bool{}
	// These two separately captured free landings use a course-specific start
	// link instead of payment variants. The JavaScript landing explicitly uses
	// the older js-basics-free start slug; it is not a general alias rule.
	startSlug := map[string]string{"java-basics-free": "java-basics-free", "javascript-basics-free": "js-basics-free"}[c.ExternalID]
	startLinks, foreignStart := 0, false
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "h1" {
			heads++
			title = nodeText(n)
		}
		if n.Data == "meta" && nodeAttr(n, "name") == "description" {
			description = nodeAttr(n, "content")
		}
		if n.Data == "script" && nodeAttr(n, "type") == "application/ld+json" {
			var v any
			if n.FirstChild == nil || json.Unmarshal([]byte(n.FirstChild.Data), &v) != nil {
				invalid = true
				return
			}
			var walk func(any)
			walk = func(v any) {
				switch v := v.(type) {
				case map[string]any:
					types := []any{v["@type"]}
					if list, ok := v["@type"].([]any); ok {
						types = list
					}
					for _, typ := range types {
						if name, ok := typ.(string); ok {
							name = strings.TrimPrefix(strings.TrimPrefix(name, "https://schema.org/"), "http://schema.org/")
							switch name {
							case "Course", "Product", "Offer", "AggregateOffer":
								invalid = true
							}
						}
					}
					for _, x := range v {
						walk(x)
					}
				case []any:
					for _, x := range v {
						walk(x)
					}
				}
			}
			walk(v)
		}
		if n.Data == "a" {
			u, e := url.Parse(nodeAttr(n, "href"))
			if e != nil || u.Scheme != "https" || u.Host != "ru.hexlet.io" || u.User != nil || u.Fragment != "" {
				return
			}
			if startSlug != "" && strings.HasPrefix(u.Path, "/programs/") && strings.HasSuffix(u.Path, "/goto") {
				if u.Path == "/programs/"+startSlug+"/goto" && u.RawQuery == "" {
					startLinks++
				} else {
					foreignStart = true
				}
			}
			q, e := url.ParseQuery(u.RawQuery)
			if e != nil {
				return
			}
			if u.Path == "/payments/new" && len(q) == 2 && len(q["program"]) == 1 && len(q["variant"]) == 1 && q.Get("program") == c.ExternalID {
				switch q.Get("variant") {
				case "standard":
					variants["base"] = true
				case "main":
					variants["general"] = true
				case "premium":
					variants["premium"] = true
				}
				return
			}
			if u.Path != "/programs/"+c.ExternalID+"/payment/new" {
				return
			}
			for key, values := range q {
				if (key != "price" && key != "payment_type") || len(values) != 1 {
					return
				}
			}
			if v, ok := q["payment_type"]; ok && v[0] != "self_payment" {
				return
			}
			price := q.Get("price")
			if price == "" && len(q) == 0 {
				price = "general"
			}
			switch price {
			case "base", "general", "premium":
				variants[price] = true
			}
		}
	})
	titleKey := func(s string) string {
		s = normalizeTitle(s)
		s = strings.TrimPrefix(s, "курс ")
		s = strings.Trim(s, "\"")
		return strings.TrimSuffix(s, " с нуля")
	}
	identityLinks := len(variants) == 3
	if startSlug != "" {
		identityLinks = startLinks > 0 && !foreignStart && len(variants) == 0
	}
	if invalid || heads != 1 || titleKey(title) != titleKey(current.Title) || courseLanguage(title, description) != current.Language || !identityLinks || description == "" {
		return nil, ErrAI
	}
	text := aiNormalize(aiPublicText(description) + "\n" + aiCourseSections(raw, c))
	if len(text) < 80 {
		return nil, ErrAI
	}
	docs := []AIDocument{{ID: "course-public", Scope: "course", Partial: true, Source: c.URL, Text: text, Digest: digest, BindingDigest: aiHexletPublicDigest(c, current.ID, aiIdentityDigest(current), b)}}
	if startSlug == "" {
		docs = append(docs, aiHexletTariffDocuments(raw, c, current, digest)...)
	}
	if len(docs) > 1 {
		for i := range docs {
			docs[i].BindingDigest = aiHexletPublicTariffDigest(c, current.ID, aiIdentityDigest(current), b)
		}
	}
	if !aiDocumentsValid(c, docs) {
		return nil, ErrAI
	}
	return docs, nil
}
func aiHexletPublicDigest(c Candidate, id, identity string, b *aiSavedBinding) string {
	raw, _ := json.Marshal(struct {
		Candidate          Candidate
		CourseID, Identity string
		Binding            *aiSavedBinding
	}{c, id, identity, b})
	return fingerprint(append([]byte("hexlet-public-course-v1\x00"), raw...))
}
func aiPartialPublicDocuments(docs []AIDocument) bool {
	return len(docs) == 1 && docs[0].ID == "course-public" && docs[0].Scope == "course" && docs[0].Partial && docs[0].OfferID == ""
}
