package updater

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"golang.org/x/net/html"
)

func aiPublicText(raw string) string {
	doc := learningDocument([]byte(raw))
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if profileBlockedNode(n) || n.Data == "form" || n.Data == "input" {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			b.WriteByte(' ')
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
		if n.Type == html.ElementNode && (n.Data == "p" || n.Data == "li" || n.Data == "h2" || n.Data == "h3" || n.Data == "section") {
			b.WriteByte('\n')
		}
	}
	walk(doc)
	return aiNormalize(b.String())
}

// Fetch only existing verified course identities. Model-suggested URLs are never fetched.
func aiFetchDocuments(ctx context.Context, db *store.DB, client *http.Client, course catalog.Course) (Candidate, []AIDocument, error) {
	c, ok := aiCandidateURL(course.Source)
	if !ok || course.Status != "published" || course.Demo {
		return c, nil, ErrAI
	}
	adapter := c.Adapter
	raw, digest, err := fetch(ctx, client, candidateEndpoint(c))
	if err != nil {
		return c, nil, err
	}
	var docs []AIDocument
	if adapter == "netology" {
		return aiFetchNetologyDocuments(ctx, db, course, c, raw, digest)
	}
	if adapter == "stepik" {
		var payload struct {
			Courses []stepikCourse `json:"courses"`
		}
		if decodeJSON(raw, &payload) != nil || len(payload.Courses) != 1 {
			return c, nil, ErrAI
		}
		remote := payload.Courses[0]
		if strconv.Itoa(remote.ID) != c.ExternalID || remote.Public == nil || !*remote.Public || remote.Active == nil || !*remote.Active || remote.Enabled == nil || !*remote.Enabled || remote.Archived == nil || *remote.Archived || remote.Censored == nil || *remote.Censored || remote.ContentLanguage != "ru" || courseLanguage(remote.Title, remote.Summary) != course.Language {
			return c, nil, ErrAI
		}
		text := "Название: " + remote.Title + "\nОписание: " + aiPublicText(remote.Summary) + "\nТребования: " + aiPublicText(remote.Requirements) + "\nАудитория: " + aiPublicText(remote.TargetAudience) + "\nНагрузка: " + aiPublicText(remote.Workload) + "\nПрограмма: " + aiPublicText(remote.Description)
		if len(aiPublicText(remote.Description)+remote.Requirements+remote.TargetAudience+remote.Workload+remote.Summary) < 80 {
			return c, nil, ErrAI
		}
		docs = append(docs, AIDocument{ID: "course", Source: candidateEndpoint(c), Scope: "course", Text: text, Digest: digest})
		// A Stepik course's sole verified full-course offer shares its official API
		// document. Independently imported additional offers receive no inferred facts.
		if len(course.Offers) == 1 && course.Offers[0].URL == c.URL {
			docs = append(docs, AIDocument{ID: "offer:" + course.Offers[0].ID, Source: candidateEndpoint(c), Scope: "offer", OfferID: course.Offers[0].ID, Text: text, Digest: digest})
		}
	} else if adapter == "yandex" {
		if practicumPage(raw, c) != nil {
			return c, nil, ErrAI
		}
		doc := learningDocument(raw)
		// Extract course text before removing verified tariff cards. Keep those cards
		// as separate offer documents, never mix Plus benefits into base suitability.
		bindings := map[string]practicumTariffBinding{}
		var primaryOffer string
		var encoded []byte
		err = db.Pool.QueryRow(ctx, "SELECT offer_id,practicum_tariffs FROM catalog_identities WHERE adapter='yandex' AND canonical_url=$1", c.URL).Scan(&primaryOffer, &encoded)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return c, nil, err
		}
		if err == nil {
			if len(encoded) > 0 && string(encoded) != "null" && json.Unmarshal(encoded, &bindings) != nil {
				return c, nil, ErrAI
			}
			if len(bindings) == 0 {
				bindings[c.ExternalID] = practicumTariffBinding{OfferID: primaryOffer}
			}
		}
		for slug, binding := range bindings {
			if !slugPattern.MatchString(slug) || slug != c.ExternalID && !boundPracticumTariff(raw, c, slug) {
				return c, nil, ErrAI
			}
			offerExists := false
			for _, o := range course.Offers {
				if o.ID == binding.OfferID && o.URL == c.URL {
					offerExists = true
				}
			}
			if !offerExists {
				continue
			}
			card := learningOne(doc, func(n *html.Node) bool {
				return classToken(n, "common-flow-card__wrapper") && nodeAttr(n, "id") == slug
			})
			if card == nil {
				continue
			}
			text := nodeText(card)
			if slug == c.ExternalID {
				base := learningOne(doc, func(n *html.Node) bool { return classToken(n, "mentors-section") })
				if base != nil {
					text += "\nБазовая программа: " + nodeText(base)
				}
			}
			docs = append(docs, AIDocument{ID: "offer:" + binding.OfferID, Source: c.URL, Scope: "offer", OfferID: binding.OfferID, Text: aiNormalize(text), Digest: digest})
		}
		// Remove every purchasable flow card from the course-level document.
		var remove []*html.Node
		visitNodes(doc, func(n *html.Node) {
			if classToken(n, "common-flow-card__wrapper") {
				remove = append(remove, n)
			}
		})
		for _, n := range remove {
			if n.Parent != nil {
				n.Parent.RemoveChild(n)
			}
		}
		var b strings.Builder
		html.Render(&b, doc)
		text := aiPublicText(b.String())
		if len(text) < 200 {
			return c, nil, ErrAI
		}
		docs = append(docs, AIDocument{ID: "course", Source: c.URL, Scope: "course", Text: text, Digest: digest})
	} else {
		return c, nil, ErrAI
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	if !aiDocumentsValid(c, docs) {
		return c, nil, ErrAI
	} // Oversized complete documents fail; never truncate.
	return c, docs, nil
}
func aiBundleDigest(docs []AIDocument) string {
	copyDocs := append([]AIDocument(nil), docs...)
	for i := range copyDocs {
		copyDocs[i].Digest = ""
		copyDocs[i].Text = aiNormalize(copyDocs[i].Text)
	}
	sort.Slice(copyDocs, func(i, j int) bool { return copyDocs[i].ID < copyDocs[j].ID })
	raw, _ := json.Marshal(copyDocs)
	return fingerprint(raw)
}
func aiEffectivePolicy() string {
	schema, _ := json.Marshal(aiExtractionSchema())
	compact := aiDeepSeekOutputInstructions(nil) + aiDeepSeekOutputInstructions(map[string]any{"proposed_claims": true})
	return aiPolicyVersion + "-" + fingerprint(append([]byte(aiExtractionPrompt+aiVerificationPrompt+compact), schema...))[:12]
}
