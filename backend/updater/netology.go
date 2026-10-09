package updater

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

type netologyProgram struct {
	ID              int64           `json:"program_id"`
	FamilyID        int64           `json:"program_family_id"`
	Family          string          `json:"program_family_url"`
	URLCode         string          `json:"urlcode"`
	Name            string          `json:"name"`
	Title           string          `json:"title"`
	Type            string          `json:"type"`
	Alias           string          `json:"program_type_alias"`
	Active          *bool           `json:"is_active"`
	Async           *bool           `json:"is_async"`
	Started         *bool           `json:"isProgramStarted"`
	Fake            *bool           `json:"isFake"`
	Sandbox         *bool           `json:"fromSandbox"`
	Loaded          *bool           `json:"isLoadedOnServer"`
	Free            *bool           `json:"isFreeProgram"`
	Deferred        *bool           `json:"isDeferredPaymentProgram"`
	Additional      *bool           `json:"isAdditionalLessonApplied"`
	PriceType       string          `json:"price_type"`
	Price           json.RawMessage `json:"price"`
	Original        json.RawMessage `json:"price_without_discount"`
	Initial         json.RawMessage `json:"initialPrice"`
	InitialOriginal json.RawMessage `json:"initialPriceWithoutDiscount"`
	Packages        json.RawMessage `json:"resource_packages"`
	Promo           json.RawMessage `json:"promoCodeInfo"`
	DiscountDate    string          `json:"discount_finish_date"`
	Date            string          `json:"date"`
	Starts          string          `json:"starts_at"`
	Meta            struct {
		Description string `json:"description"`
	} `json:"meta"`
}
type netologyCard struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	PriceHidden *bool  `json:"isPriceHidden"`
	OrderHidden *bool  `json:"isOrderButtonHidden"`
	Redirect    string `json:"customRedirect"`
}
type netologyPlan struct {
	Sections []struct {
		Cards []netologyCard `json:"cards"`
	} `json:"sections"`
}
type netologyPage struct {
	Page     string `json:"page"`
	Fallback *bool  `json:"isFallback"`
	Query    struct {
		Name string `json:"programName"`
	} `json:"query"`
	Props struct {
		PageProps struct {
			Active *bool `json:"isActive"`
			State  struct {
				Program struct {
					Data netologyProgram `json:"data"`
				} `json:"program"`
				Landing struct {
					Family   string                     `json:"programFamilyUrl"`
					Template string                     `json:"templateType"`
					Content  map[string]json.RawMessage `json:"content"`
				} `json:"landingContent"`
			} `json:"initialState"`
		} `json:"pageProps"`
	} `json:"props"`
}

func nodeAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func classPrefix(n *html.Node, prefix string) bool {
	for _, c := range strings.Fields(nodeAttr(n, "class")) {
		if strings.HasPrefix(c, prefix) {
			return true
		}
	}
	return false
}
func visitNodes(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		visitNodes(c, fn)
	}
}
func nodeText(n *html.Node) string {
	var out strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "script" || n.Data == "style" || n.Data == "svg" || n.Data == "input" || n.Data == "template" || n.Data == "noscript") {
			return
		}
		if n.Type == html.TextNode {
			out.WriteString(n.Data)
			out.WriteByte(' ')
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(out.String()), " ")
}

var cardField = regexp.MustCompile(`^sections\[([0-9]+)\]\.cards\[([0-9]+)\]\.title$`)
var fullAmount = regexp.MustCompile(`^или ([0-9][0-9 ]*(?:[.,][0-9]{1,2})?) ₽$`)

// Bind the one-payment amount to the named tariff field in its own card, not to
// a recommendation, global payment placeholder or another program's price.
func netologyPayment(doc *html.Node, plans []netologyPlan, slug string, price int64) (string, error) {
	accepted := map[string]bool{}
	bad := false
	visitNodes(doc, func(payment *html.Node) {
		if !classPrefix(payment, "styles_prices__") {
			return
		}
		var titles, values []*html.Node
		visitNodes(payment, func(n *html.Node) {
			if classPrefix(n, "styles_paymentTitle__") {
				titles = append(titles, n)
			}
			if classPrefix(n, "styles_price__") {
				values = append(values, n)
			}
		})
		if len(titles) != 1 || len(values) != 1 || !strings.HasPrefix(nodeText(titles[0]), "одним платежом") {
			return
		}
		var fields []*html.Node
		var root *html.Node
		for parent := payment.Parent; parent != nil; parent = parent.Parent {
			fields = nil
			visitNodes(parent, func(n *html.Node) {
				if n.Data == "p" && cardField.MatchString(nodeAttr(n, "name")) {
					fields = append(fields, n)
				}
			})
			if len(fields) > 1 {
				return
			}
			if len(fields) == 1 {
				root = parent
				break
			}
		}
		if root == nil {
			return
		}
		indexes := cardField.FindStringSubmatch(nodeAttr(fields[0], "name"))
		section, sectionErr := strconv.Atoi(indexes[1])
		index, indexErr := strconv.Atoi(indexes[2])
		if sectionErr != nil || indexErr != nil {
			bad = true
			return
		}
		var shown []string
		visitNodes(root, func(n *html.Node) {
			if !classPrefix(n, "styles_title__") {
				return
			}
			found := false
			visitNodes(n, func(c *html.Node) {
				if c == fields[0] {
					found = true
				}
			})
			if found {
				shown = append(shown, nodeText(n))
			}
		})
		if len(shown) != 1 {
			return
		}
		matching := map[string]netologyCard{}
		for _, plan := range plans {
			if section >= len(plan.Sections) || index >= len(plan.Sections[section].Cards) {
				continue
			}
			card := plan.Sections[section].Cards[index]
			card.Title = plainText(card.Title, 300)
			if card.Title != "" && normalizeTitle(card.Title) == normalizeTitle(shown[0]) {
				if previous, exists := matching[card.Slug]; exists {
					a, _ := json.Marshal(previous)
					b, _ := json.Marshal(card)
					if !bytes.Equal(a, b) {
						bad = true
						return
					}
				}
				matching[card.Slug] = card
			}
		}
		card, ok := matching[slug]
		if !ok {
			return
		}
		if len(matching) != 1 || card.PriceHidden == nil || *card.PriceHidden || card.OrderHidden == nil || *card.OrderHidden || card.Redirect != "" {
			bad = true
			return
		}
		match := fullAmount.FindStringSubmatch(nodeText(values[0]))
		if len(match) != 2 {
			bad = true
			return
		}
		amount, err := rubles([]byte(strings.ReplaceAll(strings.ReplaceAll(match[1], " ", ""), ",", ".")))
		if err != nil || amount != price {
			bad = true
			return
		}
		accepted[plainText(card.Title, 300)] = true
	})
	if bad || len(accepted) != 1 {
		return "", rejection("unverified_netology_payment")
	}
	for name := range accepted {
		return name, nil
	}
	return "", ErrSource
}

var russianMonths = map[string]time.Month{"января": 1, "февраля": 2, "марта": 3, "апреля": 4, "мая": 5, "июня": 6, "июля": 7, "августа": 8, "сентября": 9, "октября": 10, "ноября": 11, "декабря": 12}

func netologyStart(raw string) (time.Time, error) {
	parts := strings.Fields(raw)
	if len(parts) != 3 {
		return time.Time{}, ErrSource
	}
	day, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, ErrSource
	}
	year, err := strconv.Atoi(parts[2])
	if err != nil || year < 2000 || year > 2100 {
		return time.Time{}, ErrSource
	}
	month, ok := russianMonths[parts[1]]
	if !ok {
		return time.Time{}, ErrSource
	}
	date := time.Date(year, month, day, 0, 0, 0, 0, time.FixedZone("Europe/Moscow", 10800))
	if date.Day() != day || date.Month() != month {
		return time.Time{}, ErrSource
	}
	return date, nil
}
func collectNetology(data []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	var empty catalog.Course
	var o Observation
	identity, ok := candidateURL(c.Adapter, c.URL, c.FeedID)
	if len(data) > maxBody || c.Adapter != "netology" || !ok || identity.URL != c.URL || identity.ExternalID != c.ExternalID {
		return empty, o, rejection("identity_mismatch")
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return empty, o, rejection("invalid_netology_page")
	}
	canonicals, scripts := 0, 0
	russian, canonicalOK := false, false
	var payload []byte
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "html" && (nodeAttr(n, "lang") == "ru" || nodeAttr(n, "lang") == "ru-RU") {
			russian = true
		}
		if n.Data == "link" && nodeAttr(n, "rel") == "canonical" {
			canonicals++
			canonicalOK = matchingURL(nodeAttr(n, "href"), c.URL)
		}
		if n.Data == "script" && nodeAttr(n, "id") == "__NEXT_DATA__" {
			scripts++
			if nodeAttr(n, "type") == "application/json" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				payload = []byte(n.FirstChild.Data)
			}
		}
	})
	if !russian || canonicals != 1 || !canonicalOK || scripts != 1 {
		return empty, o, rejection("invalid_netology_page")
	}
	var page netologyPage
	if decodeJSON(payload, &page) != nil || page.Page != "/programs/[programName]" || page.Query.Name != c.ExternalID || page.Fallback == nil || *page.Fallback {
		return empty, o, rejection("invalid_netology_payload")
	}
	p := page.Props.PageProps.State.Program.Data
	l := page.Props.PageProps.State.Landing
	if p.ID <= 0 || p.FamilyID <= 0 || p.Family != c.ExternalID || p.Name != c.ExternalID || !slugPattern.MatchString(p.URLCode) || l.Family != c.ExternalID || l.Template != "courseLanding" {
		return empty, o, rejection("identity_mismatch")
	}
	if page.Props.PageProps.Active == nil || !*page.Props.PageProps.Active || p.Active == nil || !*p.Active || p.Loaded == nil || !*p.Loaded || p.Fake == nil || *p.Fake || p.Sandbox == nil || *p.Sandbox || p.Free == nil || *p.Free || p.Deferred == nil || *p.Deferred || p.Additional == nil || *p.Additional || p.PriceType != "paid" || (p.Type != "profession" && p.Type != "course") || p.Alias != p.Type || !emptyJSON(p.Packages, "[]") || !emptyJSON(p.Promo, "{}") {
		return empty, o, rejection("invalid_netology_payload")
	}
	price, err := rubles(p.Price)
	if err != nil || price <= 0 {
		return empty, o, rejection("invalid_netology_price")
	}
	original, err := rubles(p.Original)
	if err != nil || original < price {
		return empty, o, rejection("invalid_netology_price")
	}
	initial, err := rubles(p.Initial)
	if err != nil || initial != price {
		return empty, o, rejection("invalid_netology_price")
	}
	initialOriginal, err := rubles(p.InitialOriginal)
	if err != nil || initialOriginal != original {
		return empty, o, rejection("invalid_netology_price")
	}
	if price < original {
		deadline, err := time.ParseInLocation("2006-01-02", p.DiscountDate, time.FixedZone("Europe/Moscow", 10800))
		if err != nil || !now.Before(deadline.AddDate(0, 0, 1)) {
			return empty, o, rejection("invalid_netology_price")
		}
		deadline = deadline.AddDate(0, 0, 1)
		o.ValidUntil = &deadline
	}
	var plans []netologyPlan
	for key, raw := range l.Content {
		if strings.HasPrefix(key, "plansNew_") {
			var plan netologyPlan
			if decodeJSON(raw, &plan) != nil {
				return empty, o, rejection("invalid_netology_payload")
			}
			plans = append(plans, plan)
		}
	}
	name, err := netologyPayment(doc, plans, c.ExternalID, price)
	if err != nil {
		return empty, o, err
	}
	if p.Async == nil || p.Started == nil {
		return empty, o, rejection("unverified_enrollment")
	}
	if *p.Async {
		if p.Date != "" || (normalizeTitle(p.Starts) != "в любое время" && p.Starts != "") {
			return empty, o, rejection("unverified_enrollment")
		}
		o.Enrollment = "continuous"
		o.Schedule = "flexible"
	} else {
		start, err := netologyStart(p.Starts)
		if err != nil || start.Format("2006-01-02") != p.Date {
			return empty, o, rejection("unverified_enrollment")
		}
		o.Schedule = "scheduled"
		o.Enrollment = "closed"
		if !*p.Started && now.Before(start) {
			o.Enrollment = "open"
			deadline := start
			if o.ValidUntil == nil || deadline.Before(*o.ValidUntil) {
				o.ValidUntil = &deadline
			}
		}
	}
	o.Price = &price
	o.NetologyFamilyID = p.FamilyID
	o.NetologyProgramID = p.ID
	record, err := normalizedRecord(c, p.Title, p.Meta.Description, "Нетология", o, now)
	if err == nil && languageHint(c.ExternalID) != "" && record.Language != languageHint(c.ExternalID) {
		return empty, o, rejection("identity_mismatch")
	}
	if err == nil {
		record.Offers[0].Name = name
	}
	return record, o, err
}
