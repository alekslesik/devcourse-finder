package updater

import (
	"context"
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"net/http"
	"net/url"
	"strings"
)

// Public metadata identity does not depend on a promotion/cohort still being
// current. The canonical API product and all purchase hooks must nevertheless
// match the saved binding. No prices, campaign dates or offer text are output.
func aiFetchSkillfactoryPublic(ctx context.Context, client *http.Client, raw []byte, c Candidate, current catalog.Course, b aiSavedBinding, digest string) ([]AIDocument, error) {
	pricing, pd, err := fetch(ctx, client, "https://tools.skillfactory.ru/api/get-info-by-url?url="+url.QueryEscape(c.URL+"/"))
	if err != nil {
		return nil, err
	}
	return aiSkillfactoryPublic(raw, pricing, c, current, b, fingerprint([]byte(digest+":"+pd)))
}
func aiSkillfactoryTitle(s string) string {
	s = normalizeTitle(s)
	for _, prefix := range []string{"профессия ", "курс "} {
		s = strings.TrimPrefix(s, prefix)
	}
	return strings.TrimSuffix(s, " с нуля")
}
func aiSkillfactoryPublic(raw, pricing []byte, c Candidate, current catalog.Course, b aiSavedBinding, digest string) ([]AIDocument, error) {
	if c.Adapter != "skillfactory" || !aiSourceMatches(current.Source, c) || b.Verified == nil {
		return nil, ErrAI
	}
	doc, err := rsDocument(raw, c.URL)
	if err != nil {
		return nil, ErrAI
	}
	var rows []struct {
		Tag   string `json:"tag"`
		Title string `json:"full_course_name"`
		URL   string `json:"url"`
	}
	if decodeJSON(pricing, &rows) != nil || len(rows) != 1 {
		return nil, ErrAI
	}
	p := rows[0]
	if p.Tag == "" || len(p.Tag) > 30 || !matchingURL(p.URL, c.URL) || b.Verified.CourseID != int64Stable(p.Tag) || len(b.Verified.Tariffs) != 3 {
		return nil, ErrAI
	}
	title, description := "", ""
	heads := 0
	script := false
	popups := map[string]int{}
	links := map[string]int{}
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "h1" {
			heads++
			title = nodeText(n)
		}
		if n.Data == "meta" && nodeAttr(n, "name") == "description" {
			description = nodeAttr(n, "content")
		}
		if n.Data == "script" && nodeAttr(n, "src") == "https://tools.skillfactory.ru/js/price-automation.js" {
			script = true
		}
		if n.Data == "div" && nodeAttr(n, "class") == "t-popup" {
			popups[nodeAttr(n, "data-tooltip-hook")]++
		}
		if n.Data == "a" {
			links[nodeAttr(n, "href")]++
		}
	})
	if heads != 1 || !script || description == "" || aiSkillfactoryTitle(title) != aiSkillfactoryTitle(p.Title) || aiSkillfactoryTitle(title) != aiSkillfactoryTitle(current.Title) || courseLanguage(title, description) != current.Language {
		return nil, ErrAI
	}
	for i, t := range []struct{ key, hook string }{{"basic", "BASIC"}, {"consult", "PERSONAL"}, {"vip", "PERSONAL+"}} {
		hook := "#popup:" + p.Tag + "-" + t.hook
		if b.Verified.Tariffs[t.key] != int64(i+1) || popups[hook] != 1 || links[hook] < 1 {
			return nil, ErrAI
		}
	}
	bound := false
	for _, offer := range current.Offers {
		if offer.ID == b.OfferID && aiSourceMatches(offer.URL, c) {
			bound = true
		}
	}
	if !bound {
		return nil, ErrAI
	}
	text := aiNormalize(aiPublicText(description) + "\n" + aiCourseSections(raw, c))
	if len(text) < 80 {
		return nil, ErrAI
	}
	docs := []AIDocument{{ID: "course-public", Scope: "course", Partial: true, Source: c.URL, Text: text, Digest: digest, BindingDigest: aiSkillfactoryPublicDigest(c, current.ID, b)}}
	if !aiDocumentsValid(c, docs) {
		return nil, ErrAI
	}
	return docs, nil
}
func aiSkillfactoryPublicDigest(c Candidate, id string, b aiSavedBinding) string {
	return fingerprint([]byte("skillfactory-public-course-v1\x00" + aiBoundDigest(c, id, b)))
}
