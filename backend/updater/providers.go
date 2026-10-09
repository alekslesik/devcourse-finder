package updater

import (
	"encoding/json"
	"strconv"
	"time"

	"devcourse-finder/catalog"
)

var providerNames = map[string]string{"stepik": "Stepik", "otus": "OTUS", "yandex": "Яндекс Практикум", "hexlet": "Хекслет", "codebasics": "CodeBasics"}

func candidateEndpoint(c Candidate) string {
	if c.Adapter == "stepik" {
		return "https://stepik.org/api/courses/" + c.ExternalID
	}
	if c.Adapter == "otus" || c.Adapter == "yandex" {
		return c.URL + "/"
	}
	return c.URL
}
func collectCandidate(data []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	if identity, ok := candidateURL(c.Adapter, c.URL, c.FeedID); !ok || identity.ExternalID != c.ExternalID {
		return catalog.Course{}, Observation{}, rejection("identity_mismatch")
	}
	if c.Adapter == "stepik" {
		var payload struct {
			Courses []stepikCourse `json:"courses"`
		}
		if decodeJSON(data, &payload) != nil || len(payload.Courses) != 1 {
			return catalog.Course{}, Observation{}, rejection("invalid_payload")
		}
		remote := payload.Courses[0]
		id, err := strconv.Atoi(c.ExternalID)
		if err != nil || remote.ID != id {
			return catalog.Course{}, Observation{}, rejection("identity_mismatch")
		}
		if remote.ContentLanguage != "ru" {
			return catalog.Course{}, Observation{}, rejection("unsupported_content_language")
		}
		if remote.Lessons < 3 || remote.Units < 3 {
			return catalog.Course{}, Observation{}, rejection("insufficient_curriculum")
		}
		o, err := parseStepik(data, Source{RemoteID: id, ExpectedTitle: remote.Title}, now)
		if err != nil {
			return catalog.Course{}, o, err
		}
		record, err := normalizedRecord(c, remote.Title, remote.Summary, "Stepik", o, now)
		return record, o, err
	}
	return collectStructured(data, c, now)
}
func graphNodes(value any, out *[]map[string]any) {
	switch v := value.(type) {
	case []any:
		for _, child := range v {
			graphNodes(child, out)
		}
	case map[string]any:
		if graph, ok := v["@graph"]; ok {
			graphNodes(graph, out)
		}
		kind, _ := v["@type"].(string)
		if kind == "Course" || kind == "Product" {
			*out = append(*out, v)
		}
	}
}
func schemaString(node map[string]any, key string) string { v, _ := node[key].(string); return v }
func matchingURL(raw, expected string) bool {
	clean, err := canonical(raw)
	return err == nil && clean == expected
}
func collectStructured(data []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	var nodes []map[string]any
	for _, match := range jsonLD.FindAllSubmatch(data, -1) {
		var value any
		if decodeJSON(match[1], &value) != nil {
			return catalog.Course{}, Observation{}, rejection("invalid_structured_payload")
		}
		graphNodes(value, &nodes)
	}
	type verified struct {
		node map[string]any
		o    Observation
	}
	var accepted []verified
	for _, node := range nodes {
		if node["@type"] == "Product" && node["category"] != "Online Course" {
			continue
		}
		var offer map[string]any
		switch v := node["offers"].(type) {
		case map[string]any:
			offer = v
		case []any:
			if len(v) == 1 {
				offer, _ = v[0].(map[string]any)
			}
		}
		if offer == nil {
			continue
		}
		ownURL := schemaString(node, "url")
		if ownURL == "" {
			ownURL = schemaString(offer, "url")
		}
		if !matchingURL(ownURL, c.URL) {
			continue
		}
		if u := schemaString(offer, "url"); u != "" && !matchingURL(u, c.URL) {
			continue
		}
		// An explicitly recurring price is not the cost of the full program.
		if _, ok := offer["priceSpecification"]; ok {
			continue
		}
		if _, ok := offer["eligibleDuration"]; ok {
			continue
		}
		if _, ok := offer["billingDuration"]; ok {
			continue
		}
		o := Observation{}
		if raw := schemaString(offer, "priceValidUntil"); raw != "" {
			date, err := time.Parse(time.RFC3339, raw)
			if err != nil {
				date, err = time.Parse("2006-01-02", raw)
			}
			if err != nil || !now.Before(date) {
				continue
			}
			o.ValidUntil = &date
		}
		switch schemaString(offer, "availability") {
		case "https://schema.org/InStock", "http://schema.org/InStock":
			o.Enrollment = "open"
		case "https://schema.org/OutOfStock", "http://schema.org/OutOfStock":
			o.Enrollment = "closed"
		default:
			continue
		}
		currency := schemaString(offer, "priceCurrency")
		if offer["@type"] == "Offer" {
			raw, _ := json.Marshal(offer["price"])
			price, err := rubles(raw)
			if err != nil {
				continue
			}
			if price == 0 && node["isAccessibleForFree"] != true {
				continue
			}
			if price > 0 && node["isAccessibleForFree"] == true {
				continue
			}
			if currency == "RUB" {
				// Positive JSON-LD amounts from providers whose full-payment
				// contract is not verified may be monthly subscription prices.
				// Keep paid evidence, but do not advertise an exact full amount.
				if price > 0 && c.Adapter != "otus" {
					o.PriceUnknown = true
				} else {
					o.Price = &price
				}
			} else if price > 0 && currency != "" {
				o.PriceUnknown = true
			} else {
				continue
			}
		} else if offer["@type"] == "AggregateOffer" {
			// A minimum/starting price is evidence that the program is paid, never
			// an exact full-course price. All individual tariffs remain distinct.
			raw, _ := json.Marshal(offer["lowPrice"])
			price, err := rubles(raw)
			if err != nil || price <= 0 || currency == "" {
				continue
			}
			o.PriceUnknown = true
		} else {
			continue
		}
		if instance, ok := node["hasCourseInstance"].(map[string]any); ok && instance["courseMode"] == "Online" {
			o.Schedule = "scheduled"
		}
		if c.Adapter == "codebasics" && o.Price != nil && *o.Price == 0 {
			o.Schedule = "flexible"
			if o.Enrollment == "open" {
				o.Enrollment = "continuous"
			}
		}
		accepted = append(accepted, verified{node, o})
	}
	if len(nodes) == 0 {
		return catalog.Course{}, Observation{}, rejection("missing_course_schema")
	}
	if len(accepted) == 0 {
		return catalog.Course{}, Observation{}, rejection("unverified_course_offer")
	}
	if len(accepted) != 1 {
		return catalog.Course{}, Observation{}, rejection("ambiguous_course_schema")
	}
	v := accepted[0]
	record, err := normalizedRecord(c, schemaString(v.node, "name"), schemaString(v.node, "description"), providerNames[c.Adapter], v.o, now)
	return record, v.o, err
}
