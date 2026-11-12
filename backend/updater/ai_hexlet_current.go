package updater

import (
	"encoding/json"
	"net/url"
	"strings"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

func hexletCurrentTitle(s string) string {
	s = normalizeTitle(s)
	s = strings.TrimPrefix(s, "курс ")
	s = strings.Trim(s, "\"«»„“” ")
	return strings.TrimSuffix(s, " с нуля")
}

// The current four public landings have course-specific FAQ plus payment/start
// links; three additionally publish a Course schema. Validate that schema rather
// than treating its mere presence as either success or an excuse to ignore it.
// Only own learning/suitability text enters this partial course contract.
func aiHexletCurrent(raw []byte, c Candidate, current catalog.Course, b *aiSavedBinding, digest string) ([]AIDocument, error) {
	questions := map[string][]string{
		"java":                   {"Что, если я совсем ничего не знаю о программировании?", "Какие навыки у меня будут после курса Java-разработчик?"},
		"frontend":               {"Программирование — темный лес для меня. Справлюсь?", "Какие навыки у меня будут после курса Фронтенд-разработчик?"},
		"java-basics-free":       {"Какой нужен уровень знаний в программировании?"},
		"javascript-basics-free": {"Какой нужен уровень знаний в программировании?"},
	}
	allowed, known := questions[c.ExternalID]
	if !known || c.Adapter != "hexlet" || len(raw) > maxBody || !aiSourceMatches(current.Source, c) {
		return nil, ErrAI
	}
	doc, err := rsDocument(raw, c.URL)
	if err != nil {
		return nil, ErrAI
	}
	h1s, descriptions, faqCount := 0, 0, 0
	title, description := "", ""
	invalid := false
	var courses []map[string]any
	var texts []string
	selected := map[string]bool{}
	variants := map[string]bool{}
	startSlug := map[string]string{"java-basics-free": "java-basics-free", "javascript-basics-free": "js-basics-free"}[c.ExternalID]
	starts, foreignStart := 0, false
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "h1" {
			h1s++
			title = nodeText(n)
		}
		if n.Data == "meta" && nodeAttr(n, "name") == "description" {
			descriptions++
			description = nodeAttr(n, "content")
		}
		if n.Data == "a" {
			target, e := url.Parse(nodeAttr(n, "href"))
			if e == nil && target.Scheme == "https" && target.Host == "ru.hexlet.io" && target.User == nil && target.Fragment == "" {
				if variant := aiHexletPaymentVariant(target.String(), c); variant != "" {
					variants[variant] = true
				}
				if startSlug != "" && strings.HasPrefix(target.Path, "/programs/") && strings.HasSuffix(target.Path, "/goto") {
					if target.Path == "/programs/"+startSlug+"/goto" && target.RawQuery == "" {
						starts++
					} else {
						foreignStart = true
					}
				}
			}
		}
		if n.Data != "script" || nodeAttr(n, "type") != "application/ld+json" {
			return
		}
		if n.FirstChild == nil {
			invalid = true
			return
		}
		var value any
		if decodeJSON([]byte(n.FirstChild.Data), &value) != nil {
			invalid = true
			return
		}
		// Reject contradictory or additional commercial entities anywhere in a graph.
		var walk func(any, bool)
		walk = func(value any, insideCourse bool) {
			switch v := value.(type) {
			case []any:
				for _, child := range v {
					walk(child, insideCourse)
				}
			case map[string]any:
				kind, _ := v["@type"].(string)
				if _, array := v["@type"].([]any); array {
					invalid = true
					return
				}
				kind = strings.TrimPrefix(strings.TrimPrefix(kind, "https://schema.org/"), "http://schema.org/")
				switch kind {
				case "Course":
					if insideCourse {
						invalid = true
					}
					courses = append(courses, v)
					insideCourse = true
				case "Product":
					invalid = true
				case "Offer", "AggregateOffer":
					if !insideCourse {
						invalid = true
					}
				case "FAQPage":
					faqCount++
					encoded, _ := json.Marshal(v)
					var faq struct {
						Questions []struct {
							Type   string `json:"@type"`
							Name   string `json:"name"`
							Answer struct {
								Type string `json:"@type"`
								Text string `json:"text"`
							} `json:"acceptedAnswer"`
						} `json:"mainEntity"`
					}
					if decodeJSON(encoded, &faq) != nil {
						invalid = true
						return
					}
					for _, q := range faq.Questions {
						for _, wanted := range allowed {
							if q.Name == wanted {
								if selected[q.Name] || q.Type != "Question" || q.Answer.Type != "Answer" || strings.TrimSpace(q.Answer.Text) == "" {
									invalid = true
									continue
								}
								selected[q.Name] = true
								texts = append(texts, q.Name, aiPublicText(q.Answer.Text))
							}
						}
					}
				}
				for _, child := range v {
					walk(child, insideCourse)
				}
			}
		}
		walk(value, false)
	})
	if invalid || h1s != 1 || descriptions != 1 || description == "" || hexletCurrentTitle(title) != hexletCurrentTitle(current.Title) || courseLanguage(title, description) != current.Language || faqCount != 1 || len(selected) != len(allowed) || len(courses) > 1 {
		return nil, ErrAI
	}
	if startSlug != "" {
		if len(courses) != 1 || starts == 0 || foreignStart || len(variants) != 0 {
			return nil, ErrAI
		}
	} else if len(variants) != 3 {
		return nil, ErrAI
	}
	if len(courses) == 1 && !hexletCurrentSchema(courses[0], c, current, startSlug != "") {
		return nil, ErrAI
	}
	text := aiNormalize(strings.Join(append([]string{aiPublicText(description)}, texts...), "\n"))
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

func hexletCurrentSchema(schema map[string]any, c Candidate, current catalog.Course, free bool) bool {
	provider, _ := schema["provider"].(map[string]any)
	if !matchingURL(schemaString(schema, "url"), c.URL) || schema["inLanguage"] != "ru" || hexletCurrentTitle(schemaString(schema, "name")) != hexletCurrentTitle(current.Title) || provider["name"] != "Хекслет" || !matchingURL(schemaString(provider, "sameAs"), "https://ru.hexlet.io") {
		return false
	}
	if free {
		offer, _ := schema["offers"].(map[string]any)
		encoded, _ := json.Marshal(offer["price"])
		price, err := rubles(encoded)
		return schema["isAccessibleForFree"] == true && offer["@type"] == "Offer" && offer["category"] == "Free" && err == nil && price == 0 && offer["priceCurrency"] == "RUB"
	}
	offers, ok := schema["offers"].([]any)
	if !ok || len(offers) != 3 || schema["isAccessibleForFree"] == true {
		return false
	}
	names := map[string]bool{}
	for _, value := range offers {
		offer, ok := value.(map[string]any)
		if !ok {
			return false
		}
		name := schemaString(offer, "name")
		if name != "Стандартный" && name != "Расширенный" && name != "Премиальный" || names[name] || offer["@type"] != "Offer" || offer["category"] != "Paid" || !matchingURL(schemaString(offer, "url"), c.URL) || offer["priceCurrency"] != "RUB" {
			return false
		}
		names[name] = true
	}
	return true
}
