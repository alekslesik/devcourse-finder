package updater

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

type PurpleTariff struct {
	ID       int64  `json:"id"`
	Category string `json:"category"`
	Name     string `json:"name"`
	Price    int64  `json:"price"`
	Review   bool   `json:"review"`
	Mentor   bool   `json:"mentor"`
}
type purpleCourse struct {
	ID       int64   `json:"id"`
	CourseID int64   `json:"courseId"`
	Alias    string  `json:"alias"`
	Title    string  `json:"title"`
	Summary  string  `json:"metaDescription"`
	Status   string  `json:"status"`
	OnSite   *bool   `json:"isOnSite"`
	Planned  *string `json:"plannedReleaseDate"`
	Tariffs  []struct {
		ID            int64             `json:"id"`
		CourseID      int64             `json:"courseId"`
		Name          string            `json:"name"`
		Type          string            `json:"type"`
		Status        string            `json:"status"`
		Price         json.RawMessage   `json:"price"`
		OldPrice      json.RawMessage   `json:"oldPrice"`
		Features      []string          `json:"features"`
		Subscriptions []json.RawMessage `json:"tariffInSubscription"`
		Properties    struct {
			Buy struct {
				Options []string `json:"options"`
			} `json:"buy"`
		} `json:"properties"`
	} `json:"tariffs"`
	Sections []struct {
		CourseID int64 `json:"courseId"`
		Lessons  []struct {
			ID      int64 `json:"id"`
			Tariffs []struct {
				LessonID int64 `json:"lessonId"`
				TariffID int64 `json:"tariffId"`
			} `json:"lessonOnTariff"`
		} `json:"lessons"`
	} `json:"sections"`
}
type purpleSchema struct {
	Type     string `json:"@type"`
	URL      string `json:"url"`
	Code     string `json:"courseCode"`
	Name     string `json:"name"`
	Language string `json:"inLanguage"`
	Offers   []struct {
		Type           string            `json:"@type"`
		Category       string            `json:"category"`
		Name           string            `json:"name"`
		Price          json.RawMessage   `json:"price"`
		Currency       string            `json:"priceCurrency"`
		URL            string            `json:"url"`
		Availability   string            `json:"availability"`
		Deadline       json.RawMessage   `json:"priceValidUntil"`
		Billing        json.RawMessage   `json:"billingDuration"`
		Eligible       json.RawMessage   `json:"eligibleDuration"`
		Specifications []json.RawMessage `json:"priceSpecification"`
	} `json:"offers"`
}

func purpleClass(n *html.Node, suffix string) bool {
	for _, c := range strings.Fields(nodeAttr(n, "class")) {
		if strings.HasPrefix(c, "TariffCardV2-module_") && strings.HasSuffix(c, "__"+suffix) {
			return true
		}
	}
	return false
}

var purpleAmount = regexp.MustCompile(`^([0-9][0-9 ]*(?:,[0-9]{1,2})?) ₽$`)

func purpleFlight(doc *html.Node, slug string) ([]byte, error) {
	var stream strings.Builder
	visitNodes(doc, func(n *html.Node) {
		if n.Data != "script" || n.FirstChild == nil {
			return
		}
		raw := strings.TrimSpace(n.FirstChild.Data)
		if !strings.HasPrefix(raw, "self.__next_f.push(") || !strings.HasSuffix(raw, ")") {
			return
		}
		var parts []json.RawMessage
		if decodeJSON([]byte(strings.TrimSuffix(strings.TrimPrefix(raw, "self.__next_f.push("), ")")), &parts) != nil || len(parts) != 2 || string(parts[0]) != "1" {
			return
		}
		var chunk string
		if json.Unmarshal(parts[1], &chunk) == nil {
			stream.WriteString(chunk)
		}
	})
	if stream.Len() > maxBody {
		return nil, ErrSource
	}
	var found [][]byte
	var walk func(any, int)
	walk = func(v any, depth int) {
		if depth > 64 {
			return
		}
		switch v := v.(type) {
		case map[string]any:
			if c, ok := v["course"].(map[string]any); ok && c["alias"] == slug {
				b, _ := json.Marshal(c)
				found = append(found, b)
			}
			for _, child := range v {
				walk(child, depth+1)
			}
		case []any:
			for _, child := range v {
				walk(child, depth+1)
			}
		}
	}
	for _, line := range strings.Split(stream.String(), "\n") {
		_, raw, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		var value any
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if json.Valid([]byte(raw)) && decoder.Decode(&value) == nil {
			walk(value, 0)
		}
	}
	if len(found) != 1 {
		return nil, ErrSource
	}
	return found[0], nil
}
func collectPurpleSchool(data []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	var empty catalog.Course
	var o Observation
	reject := func() (catalog.Course, Observation, error) {
		return empty, Observation{}, rejection("invalid_purpleschool_contract")
	}
	identity, ok := candidateURL(c.Adapter, c.URL, c.FeedID)
	if !ok || c.Adapter != "purpleschool" || identity.ExternalID != c.ExternalID || identity.URL != c.URL || len(data) > maxBody {
		return reject()
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return reject()
	}
	canonicalCount := 0
	canonicalOK := false
	russian := false
	var schemas []purpleSchema
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "html" && (nodeAttr(n, "lang") == "ru" || nodeAttr(n, "lang") == "ru-RU") {
			russian = true
		}
		if n.Data == "link" && nodeAttr(n, "rel") == "canonical" {
			canonicalCount++
			canonicalOK = matchingURL(nodeAttr(n, "href"), c.URL)
		}
		if n.Data == "script" && nodeAttr(n, "type") == "application/ld+json" && n.FirstChild != nil {
			var raw []json.RawMessage
			if json.Unmarshal([]byte(n.FirstChild.Data), &raw) != nil {
				raw = []json.RawMessage{json.RawMessage(n.FirstChild.Data)}
			}
			for _, b := range raw {
				var s purpleSchema
				if decodeJSON(b, &s) == nil && s.Type == "Course" && matchingURL(s.URL, c.URL) {
					schemas = append(schemas, s)
				}
			}
		}
	})
	if !russian || canonicalCount != 1 || !canonicalOK || len(schemas) != 1 {
		return reject()
	}
	raw, err := purpleFlight(doc, c.ExternalID)
	if err != nil {
		return reject()
	}
	var p purpleCourse
	if decodeJSON(raw, &p) != nil || p.ID <= 0 || p.CourseID != p.ID || p.Alias != c.ExternalID || p.Status != "published" || p.OnSite == nil || !*p.OnSite || p.Planned != nil {
		return reject()
	}
	s := schemas[0]
	if s.Code != p.Alias || normalizeTitle(s.Name) != normalizeTitle(p.Title) || s.Language != "ru-RU" || len(s.Offers) != len(p.Tariffs) || len(p.Tariffs) < 1 || len(p.Tariffs) > 5 {
		return reject()
	}
	deadline := now.Add(26 * time.Hour) // A verification lease, not an inferred promotion/cohort date.
	o.ValidUntil = &deadline
	o.Enrollment = "continuous"
	o.Schedule = "flexible"
	o.PurpleCourseID = p.ID
	ids := map[int64]bool{}
	categories := map[string]bool{}
	for _, t := range p.Tariffs {
		if t.ID <= 0 || t.CourseID != p.ID || t.Status != "ACTIVE" || ids[t.ID] || categories[t.Type] || len(t.Properties.Buy.Options) != 1 || t.Properties.Buy.Options[0] != "SOLO" || len(t.Subscriptions) > 0 {
			return reject()
		}
		ids[t.ID] = true
		categories[t.Type] = true
		price, err := rubles(t.Price)
		if err != nil {
			return reject()
		}
		schemaMatches := 0
		for _, offer := range s.Offers {
			u, err := url.Parse(offer.URL)
			if err != nil {
				continue
			}
			q := u.Query()
			courseID, _ := strconv.ParseInt(q.Get("course"), 10, 64)
			tariffID, _ := strconv.ParseInt(q.Get("tariffId"), 10, 64)
			if tariffID != t.ID {
				continue
			}
			schemaMatches++
			amount, err := rubles(offer.Price)
			if err != nil || amount != price || courseID != p.ID || u.Scheme != "https" || u.Host != "app.purpleschool.ru" || u.Path != "/buy" || u.User != nil || u.Fragment != "" || len(q) != 3 || len(q["course"]) != 1 || len(q["tariffId"]) != 1 || len(q["v"]) != 1 || q.Get("v") != "3" || offer.Type != "Offer" || offer.Category != t.Type || offer.Name != t.Name || offer.Currency != "RUB" || offer.Availability != "https://schema.org/InStock" || len(offer.Specifications) != 1 {
				return reject()
			}
			if len(offer.Deadline) > 0 || len(offer.Billing) > 0 || len(offer.Eligible) > 0 {
				return reject()
			}
			var spec map[string]json.RawMessage
			if decodeJSON(offer.Specifications[0], &spec) != nil || len(spec) != 3 {
				return reject()
			}
			var kind, currency string
			json.Unmarshal(spec["@type"], &kind)
			json.Unmarshal(spec["priceCurrency"], &currency)
			sp, err := rubles(spec["price"])
			if err != nil || sp != price || kind != "PriceSpecification" || currency != "RUB" {
				return reject()
			}
		}
		if schemaMatches != 1 {
			return reject()
		}
		if t.Type == "free" {
			if price != 0 {
				return reject()
			}
			if len(p.Tariffs) > 1 {
				if t.Name != "Бесплатные модули" {
					return reject()
				}
				continue
			}
			if t.Name != "Бесплатный курс" || !strings.Contains(nodeText(doc), "Полный курс - Бесплатно") {
				return reject()
			}
			lessons := map[int64]bool{}
			for _, section := range p.Sections {
				if section.CourseID != p.ID {
					return reject()
				}
				for _, lesson := range section.Lessons {
					if lesson.ID <= 0 || lessons[lesson.ID] {
						return reject()
					}
					lessons[lesson.ID] = true
					bound := false
					for _, b := range lesson.Tariffs {
						if b.LessonID == lesson.ID && b.TariffID == t.ID {
							bound = true
						}
					}
					if !bound {
						return reject()
					}
				}
			}
			if len(lessons) < 3 || !strings.Contains(nodeText(doc), "Цена: Бесплатно") {
				return reject()
			}
		} else {
			expectedName := map[string]string{"basic": "Самостоятельный", "aiMentor": "AI и тренажёры", "teamProject": "Наставник и практика"}[t.Type]
			if t.Name != expectedName {
				return reject()
			}
			if price <= 0 || (t.Type != "basic" && t.Type != "aiMentor" && t.Type != "teamProject") {
				return reject()
			}
			old, err := rubles(t.OldPrice)
			if err != nil || old < price {
				return reject()
			}
			matches := 0
			valid := true
			visitNodes(doc, func(n *html.Node) {
				if !purpleClass(n, "card") {
					return
				}
				var names, prices, originals []string
				action := false
				visitNodes(n, func(child *html.Node) {
					if purpleClass(child, "title") {
						names = append(names, nodeText(child))
					}
					if purpleClass(child, "priceText") {
						prices = append(prices, nodeText(child))
					}
					if purpleClass(child, "oldPrice") {
						originals = append(originals, nodeText(child))
					}
					disabled := false
					for _, a := range child.Attr {
						if a.Key == "disabled" {
							disabled = true
						}
					}
					if child.Data == "button" && nodeText(child) == "Начать курс" && !disabled {
						action = true
					}
				})
				if len(names) != 1 || names[0] != t.Name {
					return
				}
				matches++
				if len(prices) != 1 || !action {
					valid = false
					return
				}
				m := purpleAmount.FindStringSubmatch(prices[0])
				if len(m) != 2 {
					valid = false
					return
				}
				actual, err := rubles([]byte(strings.ReplaceAll(strings.ReplaceAll(m[1], " ", ""), ",", ".")))
				if err != nil || actual != price {
					valid = false
				}
				if old > price {
					if len(originals) != 1 {
						valid = false
						return
					}
					m = purpleAmount.FindStringSubmatch(originals[0])
					if len(m) != 2 {
						valid = false
						return
					}
					actual, err = rubles([]byte(strings.ReplaceAll(strings.ReplaceAll(m[1], " ", ""), ",", ".")))
					if err != nil || actual != old {
						valid = false
					}
				}
			})
			if matches != 1 || !valid {
				return reject()
			}
		}
		if t.Type == "basic" && len(t.Features) != 0 {
			return reject()
		}
		if t.Type == "aiMentor" {
			features := strings.Join(t.Features, " ")
			if !strings.Contains(features, "Домашние задания с AI проверкой") || !strings.Contains(features, "Прямой чат") || !strings.Contains(features, "AI-наставником") {
				return reject()
			}
		}
		review, mentor := false, false
		if t.Type == "teamProject" {
			text := strings.Join(t.Features, " ")
			if !strings.Contains(text, "Ревью от опытных наставников") || !strings.Contains(text, "Прямой чат с менторами") {
				return reject()
			}
			review = true
			mentor = true
		}
		o.PurpleTariffs = append(o.PurpleTariffs, PurpleTariff{ID: t.ID, Category: t.Type, Name: t.Name, Price: price, Review: review, Mentor: mentor})
	}
	if len(o.PurpleTariffs) == 0 {
		return reject()
	}
	o.Price = &o.PurpleTariffs[0].Price
	record, err := normalizedRecord(c, p.Title, p.Summary, "PurpleSchool", o, now)
	if err != nil {
		return empty, Observation{}, err
	}
	record.Offers = nil
	for _, t := range o.PurpleTariffs {
		known := true
		amount := t.Price
		record.Offers = append(record.Offers, catalog.Offer{ID: record.ID + "-t" + strconv.FormatInt(t.ID, 10), Name: t.Name, Price: &amount, PriceKind: "exact", Free: amount == 0, PriceCheckedAt: now, ValidUntil: &deadline, SupportKnown: &known, Review: t.Review, Mentor: t.Mentor, Schedule: "flexible", Enrollment: "continuous", URL: c.URL})
	}
	return record, o, nil
}
