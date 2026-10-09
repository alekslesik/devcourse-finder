package updater

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

func rsNodes(root *html.Node, testID string) []*html.Node {
	var found []*html.Node
	visitNodes(root, func(n *html.Node) {
		if nodeAttr(n, "data-testid") == testID {
			found = append(found, n)
		}
	})
	return found
}
func rsDocument(data []byte, expected string) (*html.Node, error) {
	if len(data) > maxBody {
		return nil, ErrSource
	}
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, ErrSource
	}
	count := 0
	valid := false
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "link" && nodeAttr(n, "rel") == "canonical" {
			count++
			valid = matchingURL(nodeAttr(n, "href"), expected)
		}
	})
	if count != 1 || !valid {
		return nil, ErrSource
	}
	return doc, nil
}
func rsDates(root *html.Node) (time.Time, time.Time, error) {
	dates := rsNodes(root, "date-time-start")
	if len(dates) != 2 {
		return time.Time{}, time.Time{}, ErrSource
	}
	values := make([]time.Time, 2)
	for i, n := range dates {
		if n.Data != "time" {
			return time.Time{}, time.Time{}, ErrSource
		}
		value, err := time.Parse(time.RFC3339Nano, nodeAttr(n, "datetime"))
		if err != nil || nodeText(n) != value.Format("Jan 02, 2006") {
			return time.Time{}, time.Time{}, ErrSource
		}
		values[i] = value
	}
	if values[1].Before(values[0]) || values[1].Sub(values[0]) > 45*24*time.Hour {
		return time.Time{}, time.Time{}, ErrSource
	}
	return values[0], values[1], nil
}
func rsLanguage(root *html.Node) string {
	n := rsNodes(root, "course-language")
	if len(n) != 1 {
		return ""
	}
	return nodeText(n[0])
}
func collectRSSchool(data, catalogData []byte, c Candidate, now time.Time) (catalog.Course, Observation, error) {
	var empty catalog.Course
	var o Observation
	reject := func(code string) (catalog.Course, Observation, error) { return empty, Observation{}, rejection(code) }
	identity, ok := candidateURL(c.Adapter, c.URL, c.FeedID)
	if c.Adapter != "rsschool" || !ok || identity.URL != c.URL || identity.ExternalID != c.ExternalID {
		return reject("identity_mismatch")
	}
	doc, err := rsDocument(data, c.URL)
	if err != nil {
		return reject("invalid_rsschool_contract")
	}
	index, err := rsDocument(catalogData, "https://rs.school/courses")
	if err != nil {
		return reject("invalid_rsschool_contract")
	}
	listings := rsNodes(index, "all-courses")
	if len(listings) != 1 {
		return reject("invalid_rsschool_contract")
	}
	var cards []*html.Node
	for _, card := range rsNodes(listings[0], "course-card") {
		links := rsNodes(card, "course-link")
		if len(links) != 1 {
			continue
		}
		if nodeAttr(links[0], "href") == "/courses/"+c.ExternalID {
			cards = append(cards, card)
		}
	}
	heroes := rsNodes(doc, "hero-course")
	if len(heroes) != 1 || len(cards) != 1 {
		return reject("identity_mismatch")
	}
	hero, card := heroes[0], cards[0]
	language := rsLanguage(hero)
	if language != rsLanguage(card) || (language != "Русский" && language != "English, Русский") {
		return reject("unsupported_content_language")
	}
	start, end, err := rsDates(hero)
	if err != nil {
		return reject("unverified_enrollment")
	}
	indexStart, indexEnd, err := rsDates(card)
	if err != nil || !start.Equal(indexStart) || !end.Equal(indexEnd) {
		return reject("unverified_enrollment")
	}
	names := rsNodes(card, "subtitle")
	titles := rsNodes(hero, "main-title")
	if len(names) != 1 || len(titles) != 1 || nodeText(titles[0]) != nodeText(names[0])+" Course" {
		return reject("identity_mismatch")
	}
	labels := rsNodes(hero, "course-label")
	if len(labels) != 1 || nodeText(labels[0]) != "available" {
		return reject("unverified_enrollment")
	}
	// A stale 'available' badge or old registration link never overrides the date.
	enrollments := map[string]bool{}
	visitNodes(hero, func(n *html.Node) {
		if n.Data != "a" || !strings.HasPrefix(nodeText(n), "Enroll") {
			return
		}
		u, err := url.Parse(nodeAttr(n, "href"))
		if err != nil {
			return
		}
		if u.Scheme == "https" && u.Host == "wearecommunity.io" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && strings.HasPrefix(u.Path, "/events/") && slugPattern.MatchString(strings.TrimPrefix(u.Path, "/events/")) {
			enrollments[u.String()] = true
		}
	})
	if len(enrollments) != 1 {
		return reject("unverified_enrollment")
	}
	// Registration URL must be bound to the current course and year/quarter.
	expectedEventPrefix := map[string]string{"javascript": "js-stage1-", "javascript-preschool-ru": "js-stage0-", "reactjs": "rs-react-", "nodejs": "nodejs-", "angular": "rs-angular-", "short-track": "js-short-track-"}[c.ExternalID]
	for link := range enrollments {
		if link != "https://wearecommunity.io/events/"+expectedEventPrefix+start.Format("2006")+"q"+strconv.Itoa((int(start.Month())-1)/3+1) {
			return reject("identity_mismatch")
		}
	}
	features := rsNodes(doc, "about-course-grid")
	if len(features) != 1 {
		return reject("invalid_rsschool_contract")
	}
	free := 0
	for _, item := range rsNodes(features[0], "feature-item") {
		headings := rsNodes(item, "subtitle")
		paragraphs := rsNodes(item, "paragraph")
		if len(headings) == 1 && nodeText(headings[0]) == "Free education" && len(paragraphs) == 1 && nodeText(paragraphs[0]) == "Our training is completely free - we share our experience with you today, hoping you’ll return as a mentor to inspire and teach the next generation tomorrow" {
			free++
		}
	}
	if free != 1 {
		return reject("unverified_rsschool_free")
	}
	// Only exact official curriculum repositories count as materials. Their old
	// schedules do not establish enrollment and are never followed as dynamic URLs.
	repos := map[string]string{"javascript": "https://github.com/rolling-scopes-school/js-fe-course-en", "javascript-preschool-ru": "https://github.com/rolling-scopes-school/tasks/tree/master/stage0", "reactjs": "https://github.com/rolling-scopes-school/tasks/tree/master/react", "nodejs": "https://github.com/rolling-scopes-school/tasks/tree/master/node", "angular": "https://github.com/rolling-scopes-school/tasks/tree/master/angular", "short-track": "https://github.com/rolling-scopes-school/tasks/tree/master/short-track"}
	curriculum := false
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "a" && nodeAttr(n, "href") == repos[c.ExternalID] {
			curriculum = true
		}
	})
	if !curriculum {
		return reject("invalid_rsschool_contract")
	}
	var summary []string
	visitNodes(doc, func(n *html.Node) {
		if n.Data != "section" {
			return
		}
		text := nodeText(n)
		if strings.HasPrefix(text, "What you should know before starting") {
			summary = append([]string{plainText(text, 550)}, summary...)
		}
		if strings.HasPrefix(text, "Training Program") {
			summary = append(summary, plainText(text, 750))
		}
	})
	for _, stage := range rsNodes(doc, "learning-path-stage-item") {
		summary = append(summary, plainText(nodeText(stage), 450))
	}
	if len(summary) == 0 {
		return reject("invalid_rsschool_contract")
	}
	price := int64(0)
	o.Price = &price
	o.Schedule = "scheduled"
	o.Enrollment = "closed"
	lease := now.Add(26 * time.Hour)
	if now.Before(end) {
		o.Enrollment = "open"
		if end.Before(lease) {
			lease = end
		}
	}
	o.ValidUntil = &lease
	record, err := normalizedRecord(c, nodeText(names[0]), strings.Join(summary, " "), "RS School", o, now)
	if err != nil {
		return empty, o, err
	}
	if record.Language != "javascript" {
		return reject("unsupported_or_ambiguous_language")
	}
	// Mentoring appears in conditional later stages. Do not promise it for every
	// entrant or convert peer cross-checking into human mentor support.
	record.Offers[0].Name = "Полная программа — бесплатное обучение"
	return record, o, nil
}
