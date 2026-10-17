package updater

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
	"golang.org/x/net/html"
)

func aiFetchCodeBasicsDocuments(ctx context.Context, db *store.DB, client *http.Client, current catalog.Course, c Candidate, raw []byte, digest string) (Candidate, []AIDocument, error) {
	if db == nil {
		return c, nil, ErrAI
	}
	var offerID string
	if err := db.Pool.QueryRow(ctx, `SELECT offer_id FROM catalog_identities WHERE adapter='codebasics' AND external_id=$1 AND canonical_url=$2 AND course_id=$3`, c.ExternalID, c.URL, current.ID).Scan(&offerID); err != nil {
		return c, nil, ErrAI
	}
	policy, _, err := fetch(ctx, client, "https://code-basics.com/ru")
	if err != nil {
		return c, nil, err
	}
	docs, err := aiCodeBasicsDocuments(raw, policy, c, current, offerID, digest, time.Now().UTC())
	return c, docs, err
}

func aiCodeBasicsBindingDigest(c Candidate, courseID, offerID string) string {
	return fingerprint([]byte(fmt.Sprintf("%s\x00%s\x00%s\x00%s", c.Adapter, c.URL, courseID, offerID)))
}

func aiCodeBasicsDocuments(raw, policy []byte, c Candidate, current catalog.Course, offerID, digest string, at time.Time) ([]AIDocument, error) {
	verified, _, err := collectCodeBasics(raw, policy, c, at)
	if err != nil || !aiSourceMatches(current.Source, c) || current.Language != verified.Language || normalizeTitle(current.Title) != normalizeTitle(verified.Title) {
		return nil, ErrAI
	}
	found := false
	for _, o := range current.Offers {
		if o.ID == offerID && aiSourceMatches(o.URL, c) && (normalizeTitle(o.Name) == normalizeTitle(verified.Offers[0].Name) || o.ID == current.ID+"-course" && o.Name == "Бесплатная полная программа" && o.Free && o.Price != nil && *o.Price == 0) {
			found = true
		}
	}
	if !found {
		return nil, ErrAI
	}
	var page basicsPage
	var payload []byte
	visitNodes(learningDocument(raw), func(n *html.Node) {
		if n.Data == "script" && nodeAttr(n, "data-page") == "app" && n.FirstChild != nil {
			payload = []byte(n.FirstChild.Data)
		}
	})
	if decodeJSON(payload, &page) != nil {
		return nil, ErrAI
	}
	p := page.Props.Landing
	// Read only explicit public course fields. Auth/user/contact state, other
	// language landings, source/test code and lesson solutions never enter prompts.
	texts := []string{
		"Название курса: " + p.Header,
		"Описание курса: " + p.Description,
		"Применение языка: " + p.UsedInHeader + " " + p.UsedInDescription,
		"Результаты обучения: " + p.OutcomesHeader + " " + p.OutcomesDescription,
	}
	for _, module := range page.Props.Modules {
		texts = append(texts, "Модуль: "+module.Name, module.Description)
		for _, lesson := range page.Props.Lessons[strconv.Itoa(module.ID)] {
			texts = append(texts, "Урок: "+lesson.Name, lesson.Description)
		}
	}
	clean := func(items []string) string {
		out := make([]string, 0, len(items))
		for _, text := range items {
			if text = aiPublicText(text); text != "" {
				out = append(out, text)
			}
		}
		return aiNormalize(strings.Join(out, "\n"))
	}
	courseText := clean(texts)
	offerText := clean([]string{p.Header, p.Description, p.Duration})
	if len(courseText) < 80 || offerText == "" {
		return nil, ErrAI
	}
	binding := aiCodeBasicsBindingDigest(c, current.ID, offerID)
	docs := []AIDocument{
		{ID: "course", Source: c.URL, Scope: "course", Text: courseText, Digest: digest, BindingDigest: binding},
		{ID: "offer:" + offerID, Source: c.URL, Scope: "offer", OfferID: offerID, Text: offerText, Digest: digest, BindingDigest: binding},
	}
	if !aiDocumentsValid(c, docs) {
		return nil, ErrAI
	}
	return docs, nil
}

func aiCodeBasicsEventIdentityValid(ctx context.Context, tx pgx.Tx, e AIEvent) (bool, error) {
	var offerID string
	err := tx.QueryRow(ctx, `SELECT offer_id FROM catalog_identities WHERE adapter='codebasics' AND external_id=$1 AND canonical_url=$2 AND course_id=$3`, e.Candidate.ExternalID, e.Candidate.URL, e.CourseID).Scan(&offerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	digest := aiCodeBasicsBindingDigest(e.Candidate, e.CourseID, offerID)
	for _, d := range e.Documents {
		if d.BindingDigest != digest {
			return false, nil
		}
	}
	return true, nil
}
