package updater

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

// This is public display data delivered by an anonymous HTTPS GET, not an
// authentication assertion. Never forward, log or persist its token, uid or form.
type academyPayment struct {
	Offers []struct {
		ID             string            `json:"id"`
		Title          string            `json:"title"`
		FullTitle      string            `json:"fulltitle"`
		ProductType    string            `json:"productType"`
		ProductCode    string            `json:"productCode"`
		ProductSubtype string            `json:"productSubtype"`
		Amount         json.Number       `json:"amount"`
		Currency       string            `json:"currency"`
		Dates          []json.RawMessage `json:"dates"`
		Places         json.RawMessage   `json:"places"`
		PaymentWays    []struct {
			Type string `json:"type"`
		} `json:"paymentWays"`
		Recurrent struct {
			Period string `json:"period"`
		} `json:"recurrent"`
		Order struct {
			Settings struct {
				Recurrent bool `json:"recurrent"`
			} `json:"settings"`
		} `json:"order"`
	} `json:"offers"`
}

var academyMonthly = regexp.MustCompile(`([0-9][0-9\x{00a0}\x{202f} ]*)\s*руб\. в месяц`)

func academyPage(data []byte, c Candidate) (string, string, int64, error) {
	identity, ok := candidateURL(c.Adapter, c.URL, c.FeedID)
	if c.Adapter != "htmlacademy" || !ok || identity.URL != c.URL || identity.ExternalID != c.ExternalID {
		return "", "", 0, rejection("identity_mismatch")
	}
	if !strings.HasPrefix(c.ExternalID, "intensive-") {
		return "", "", 0, rejection("unsupported_htmlacademy_format")
	}
	doc, err := rsDocument(data, c.URL)
	if err != nil {
		return "", "", 0, rejection("invalid_htmlacademy_page")
	}
	var titles, sections []*html.Node
	var summary string
	module := 0
	moduleMatches := false
	visitNodes(doc, func(n *html.Node) {

		if n.Data == "h1" {
			titles = append(titles, n)
		}
		if n.Data == "meta" && nodeAttr(n, "name") == "description" {
			summary = nodeAttr(n, "content")
		}
		if n.Data == "div" && nodeAttr(n, "class") == "intensive-weeks-section" {
			heading := ""
			visitNodes(n, func(child *html.Node) {
				if child.Data == "h3" {
					heading = nodeText(child)
				}
			})
			if heading == "Индивидуальный формат" {
				sections = append(sections, n)
			}
		}
		if n.Data == "script" && nodeAttr(n, "id") == "payment-form" {
			module++
			moduleMatches = nodeAttr(n, "data-render-id") == "pf-price" && nodeAttr(n, "data-token-url") == academyPaymentPath(c)
		}
	})
	language := ""
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "html" {
			language = nodeAttr(n, "lang")
		}
	})
	if language != "ru" {
		return "", "", 0, rejection("unsupported_content_language")
	}
	if len(titles) != 1 || len(sections) != 1 || summary == "" || module != 1 || !moduleMatches {
		return "", "", 0, rejection("invalid_htmlacademy_page")
	}
	text := nodeText(sections[0])
	if !strings.Contains(text, "Помесячная оплата, без банковских рассрочек и кредитов") || !strings.Contains(text, "в вашем ритме") {
		return "", "", 0, rejection("unsupported_htmlacademy_format")
	}
	buttons := 0
	visitNodes(sections[0], func(n *html.Node) {
		if n.Data == "button" && nodeAttr(n, "data-pay-tariff") == "individual" && nodeAttr(n, "disabled") == "" && !academyHasAttr(n, "disabled") && nodeText(n) == "Оплатить" {
			buttons++
		}
	})
	amounts := academyMonthly.FindAllStringSubmatch(text, -1)
	if buttons != 1 || len(amounts) != 1 {
		return "", "", 0, rejection("unverified_enrollment")
	}
	rate, err := strconv.ParseInt(strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "").Replace(amounts[0][1]), 10, 64)
	if err != nil || rate <= 0 || rate > 10000000 {
		return "", "", 0, rejection("invalid_htmlacademy_payment")
	}
	title := strings.TrimPrefix(nodeText(titles[0]), "Онлайн‑курс ")
	if title == nodeText(titles[0]) {
		return "", "", 0, rejection("invalid_htmlacademy_page")
	}
	return title, summary, rate, nil
}
func academyHasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}
func academyPaymentPath(c Candidate) string {
	return "/api/payment-data/intensive/" + strings.TrimPrefix(c.ExternalID, "intensive-") + "/individual"
}
func collectHTMLAcademy(data, paymentData []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	var empty catalog.Course
	title, summary, rate, err := academyPage(data, c)
	if err != nil {
		return empty, Observation{}, err
	}
	var envelope struct {
		Token string `json:"token"`
	}
	if len(paymentData) > maxBody || decodeJSON(paymentData, &envelope) != nil {
		return empty, Observation{}, rejection("invalid_htmlacademy_payment")
	}
	parts := strings.Split(envelope.Token, ".")
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return empty, Observation{}, rejection("invalid_htmlacademy_payment")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	var payment academyPayment
	if err != nil || decodeJSON(payload, &payment) != nil || len(payment.Offers) == 0 || len(payment.Offers) > 16 {
		return empty, Observation{}, rejection("invalid_htmlacademy_payment")
	}
	matches := 0
	for _, offer := range payment.Offers {
		// A profession bundle on the same course page is a different product.
		if offer.ID != "default" {
			continue
		}
		amount, err := offer.Amount.Int64()
		code, codeErr := strconv.ParseInt(offer.ProductCode, 10, 64)
		payable := false
		for _, method := range offer.PaymentWays {
			if method.Type == "ru.card.sbp" || method.Type == "ru.card.gazprombank" {
				payable = true
			}
		}
		if err != nil || codeErr != nil || code <= 0 || amount != rate || offer.Currency != "RUB" || offer.ProductType != "intensiveext" || offer.ProductSubtype != strings.TrimPrefix(c.ExternalID, "intensive-")+"-individual" || offer.Title != "Индивидуальный формат" || normalizeTitle(offer.FullTitle) != normalizeTitle("Курс «"+title+"». Индивидуальный формат") || offer.Recurrent.Period != "monthly" || !offer.Order.Settings.Recurrent || offer.Dates == nil || len(offer.Dates) != 0 || string(offer.Places) != "null" || !payable {
			return empty, Observation{}, rejection("invalid_htmlacademy_payment")
		}
		matches++
	}
	if matches != 1 {
		return empty, Observation{}, rejection("unverified_enrollment")
	}
	// No fixed total exists for this recurring product. Never multiply the monthly
	// payment by a suggested duration or borrow the full cost of a bundle.
	lease := now.Add(26 * time.Hour)
	o := Observation{PriceUnknown: true, Enrollment: "continuous", Schedule: "flexible", ValidUntil: &lease}
	record, err := normalizedRecord(c, title, summary+" Индивидуальный формат: помесячная оплата; полная стоимость зависит от продолжительности обучения и неизвестна.", "HTML Academy", o, now)
	if err != nil {
		return empty, Observation{}, err
	}
	record.Offers[0].Name = "Индивидуальный формат — помесячная оплата"
	return record, o, nil
}
func fetchHTMLAcademy(ctx context.Context, client *http.Client, data []byte, digest string, c Candidate) (catalog.Course, Observation, string, error) {
	if _, _, _, err := academyPage(data, c); err != nil {
		return catalog.Course{}, Observation{}, digest, err
	}
	// Construct from validated identity; do not follow arbitrary page/API links.
	payment, paymentDigest, err := fetch(ctx, client, "https://htmlacademy.ru"+academyPaymentPath(c))
	if err != nil {
		return catalog.Course{}, Observation{}, digest, rejection("source_unavailable")
	}
	record, o, err := collectHTMLAcademy(data, payment, c, time.Now().UTC())
	return record, o, fingerprint([]byte(digest + ":" + paymentDigest)), err
}
