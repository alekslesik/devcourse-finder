package updater

import (
	"io"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

// Source markup is normalized to text. Script/style/template/noscript content
// is excluded; source HTML is never stored as executable markup.
func plainText(raw string, limit int) string {
	if len(raw) > maxBody {
		return ""
	}
	tokenizer := html.NewTokenizer(strings.NewReader(raw))
	var out strings.Builder
	hidden := 0
	for {
		kind := tokenizer.Next()
		if kind == html.ErrorToken {
			if tokenizer.Err() != io.EOF {
				return ""
			}
			break
		}
		token := tokenizer.Token()
		switch kind {
		case html.StartTagToken:
			if hidden > 0 || token.Data == "script" || token.Data == "style" || token.Data == "template" || token.Data == "noscript" {
				hidden++
			}
		case html.EndTagToken:
			if hidden > 0 {
				hidden--
			}
			out.WriteByte(' ')
		case html.SelfClosingTagToken:
			out.WriteByte(' ')
		case html.TextToken:
			if hidden == 0 {
				out.WriteString(token.Data)
				out.WriteByte(' ')
			}
		}
	}
	value := strings.Join(strings.Fields(out.String()), " ")
	runes := []rune(value)
	if len(runes) > limit {
		return string(runes[:limit]) + "…"
	}
	return value
}

var languageWords = map[string]*regexp.Regexp{
	"go":         regexp.MustCompile(`(?i)\b(go|golang)\b|голанг`),
	"python":     regexp.MustCompile(`(?i)\bpython\b|питон`),
	"java":       regexp.MustCompile(`(?i)\bjava\b|джава`),
	"javascript": regexp.MustCompile(`(?i)\b(javascript|js|react|node\.js|typescript)\b|джаваскрипт|фронтенд`),
}

func courseLanguage(title, summary string) string {
	matched := []string{}
	for language, pattern := range languageWords {
		if pattern.MatchString(title) {
			matched = append(matched, language)
		}
	}
	if len(matched) == 1 {
		return matched[0]
	}
	if len(matched) > 1 {
		return ""
	}
	for language, pattern := range languageWords {
		if pattern.MatchString(summary) {
			matched = append(matched, language)
		}
	}
	if len(matched) == 1 {
		return matched[0]
	}
	return ""
}
func normalizedRecord(candidate Candidate, title, summary, provider string, o Observation, now time.Time) (catalog.Course, error) {
	title = plainText(title, 301)
	summary = plainText(summary, 2000)
	language := courseLanguage(title, summary)
	if !utf8.ValidString(title) || utf8.RuneCountInString(title) > 300 || title == "" || summary == "" || language == "" {
		return catalog.Course{}, ErrSource
	}
	id := candidate.Adapter + "-" + candidate.ExternalID
	if len(id) > 70 {
		id = candidate.Adapter + "-" + fingerprint([]byte(candidate.ExternalID))[:24]
	}
	direction := "basics"
	if strings.Contains(strings.ToLower(title), "backend") || strings.Contains(strings.ToLower(title), "бэкенд") {
		direction = "backend"
	}
	if strings.Contains(strings.ToLower(title), "frontend") || strings.Contains(strings.ToLower(title), "фронтенд") {
		direction = "frontend"
	}
	unknownSupport := false
	offer := catalog.Offer{ID: id + "-course", Name: "Полная программа", PriceKind: "unknown", Schedule: "unknown", Enrollment: o.Enrollment, URL: candidate.URL, SupportKnown: &unknownSupport}
	if o.Price != nil {
		offer.Price = o.Price
		offer.Free = *o.Price == 0
		offer.PriceKind = "exact"
		offer.PriceCheckedAt = now
	}
	if offer.Enrollment == "" {
		offer.Enrollment = "unknown"
	}
	if o.Schedule != "" {
		offer.Schedule = o.Schedule
	}
	if o.ValidUntil != nil {
		offer.ValidUntil = o.ValidUntil
	}
	c := catalog.Course{ID: id, Slug: id, Title: title, Provider: provider, Language: language, Direction: direction, Summary: summary, Audience: []string{"unknown"}, Goals: []string{"unknown"}, Topics: []string{}, Source: candidate.URL, CheckedAt: now, Status: "published", Offers: []catalog.Offer{offer}}
	return c, nil
}
