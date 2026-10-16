package updater

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"github.com/jackc/pgx/v5"
	"golang.org/x/net/html"
)

// Each reader opts in independently. A native parser is necessary but not enough:
// a reader must also isolate public course text from individually bound tiers.
type aiBoundReader func([]byte, Candidate, catalog.Course) (string, map[string]string, error)

var aiBoundReaders = map[string]aiBoundReader{}

type aiTierBinding struct {
	CourseID int64            `json:"course_id"`
	Tariffs  map[string]int64 `json:"tariffs"`
}
type aiSavedBinding struct {
	OfferID  string         `json:"offer_id"`
	Verified *aiTierBinding `json:"verified_binding"`
	Purple   *aiTierBinding `json:"purple_binding"`
	Skypro   int64          `json:"skypro_product_id"`
}

const aiBindingQuery = `SELECT jsonb_build_object('offer_id',offer_id,'verified_binding',verified_binding,'purple_binding',purple_binding,'skypro_product_id',coalesce(skypro_product_id,0)) FROM catalog_identities WHERE adapter=$1 AND external_id=$2 AND canonical_url=$3 AND course_id=$4`

func aiLoadBinding(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, c Candidate, courseID string) (aiSavedBinding, error) {
	var raw []byte
	var b aiSavedBinding
	err := q.QueryRow(ctx, aiBindingQuery, c.Adapter, c.ExternalID, c.URL, courseID).Scan(&raw)
	if err != nil {
		return b, err
	}
	err = json.Unmarshal(raw, &b)
	return b, err
}
func aiBoundDigest(c Candidate, courseID string, b aiSavedBinding) string {
	raw, _ := json.Marshal(struct {
		Candidate Candidate
		CourseID  string
		Binding   aiSavedBinding
	}{c, courseID, b})
	return fingerprint(raw)
}
func aiBoundEventIdentityValid(ctx context.Context, tx pgx.Tx, e AIEvent) (bool, error) {
	b, err := aiLoadBinding(ctx, tx, e.Candidate, e.CourseID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	digest := aiBoundDigest(e.Candidate, e.CourseID, b)
	for _, d := range e.Documents {
		if d.BindingDigest != digest {
			return false, nil
		}
	}
	return true, nil
}
func aiBindingMatches(c Candidate, current, remote catalog.Course, o Observation, b aiSavedBinding) bool {
	if current.ID != remote.ID || current.Source != c.URL || current.Language != remote.Language || normalizeTitle(current.Title) != normalizeTitle(remote.Title) || len(remote.Offers) == 0 {
		return false
	}
	if b.OfferID != remote.Offers[0].ID {
		return false
	}
	var tier *aiTierBinding
	var product int64
	var tariffs []PurpleTariff
	switch c.Adapter {
	case "skillbox", "skillfactory":
		tier, product, tariffs = b.Verified, o.VerifiedProductID, o.VerifiedTariffs
	case "purpleschool":
		tier, product, tariffs = b.Purple, o.PurpleCourseID, o.PurpleTariffs
	case "skypro":
		if b.Skypro <= 0 || b.Skypro != o.SkyproProductID {
			return false
		}
	}
	if product > 0 {
		if tier == nil || tier.CourseID != product || len(tier.Tariffs) != len(tariffs) {
			return false
		}
		for _, t := range tariffs {
			if tier.Tariffs[t.Category] != t.ID {
				return false
			}
		}
	} else if c.Adapter == "skillbox" || c.Adapter == "skillfactory" || c.Adapter == "purpleschool" {
		return false
	}
	for _, v := range remote.Offers {
		found := false
		for _, x := range current.Offers {
			if x.ID == v.ID && x.URL == c.URL && normalizeTitle(x.Name) == normalizeTitle(v.Name) {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func aiVerifyBoundSource(ctx context.Context, client *http.Client, raw []byte, digest string, c Candidate) (catalog.Course, Observation, string, error) {
	switch c.Adapter {
	case "skillfactory":
		return fetchSkillfactory(ctx, client, raw, digest, c)
	case "htmlacademy":
		return fetchHTMLAcademy(ctx, client, raw, digest, c)
	case "javarush":
		return fetchJavaRush(ctx, client, raw, digest, c)
	case "rsschool":
		index, id, e := fetch(ctx, client, "https://rs.school/courses")
		if e != nil {
			return catalog.Course{}, Observation{}, digest, e
		}
		r, o, e := collectRSSchool(raw, index, c, time.Now().UTC())
		return r, o, fingerprint([]byte(digest + ":" + id)), e
	default:
		r, o, e := collectCandidate(raw, c, time.Now().UTC())
		return r, o, digest, e
	}
}
func aiFetchBoundDocuments(ctx context.Context, db *store.DB, client *http.Client, current catalog.Course, c Candidate, raw []byte, digest string) (Candidate, []AIDocument, error) {
	if db == nil {
		return c, nil, ErrAI
	}
	b, e := aiLoadBinding(ctx, db.Pool, c, current.ID)
	if e != nil {
		return c, nil, e
	}
	remote, o, digest, e := aiVerifyBoundSource(ctx, client, raw, digest, c)
	if e != nil {
		return c, nil, e
	}
	docs, e := aiBuildBoundDocuments(raw, c, current, remote, o, b, digest)
	return c, docs, e
}
func aiBuildBoundDocuments(raw []byte, c Candidate, current, remote catalog.Course, o Observation, b aiSavedBinding, digest string) ([]AIDocument, error) {
	reader := aiBoundReaders[c.Adapter]
	if reader == nil || !aiBindingMatches(c, current, remote, o, b) {
		return nil, ErrAI
	}
	text, offers, e := reader(raw, c, remote)
	if e != nil || len(aiNormalize(text)) < 80 {
		return nil, ErrAI
	}
	binding := aiBoundDigest(c, current.ID, b)
	docs := []AIDocument{{ID: "course", Source: c.URL, Scope: "course", Text: aiNormalize(text), Digest: digest, BindingDigest: binding}}
	for _, offer := range remote.Offers {
		if text := aiNormalize(offers[offer.ID]); text != "" {
			docs = append(docs, AIDocument{ID: "offer:" + offer.ID, Source: c.URL, Scope: "offer", OfferID: offer.ID, Text: text, Digest: digest, BindingDigest: binding})
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].ID < docs[j].ID })
	if !aiDocumentsValid(c, docs) {
		return nil, ErrAI
	}
	return docs, nil
}

func aiCourseSections(raw []byte, c Candidate) string {
	doc := learningDocument(raw)
	// Remove unsafe descendants before profileSection's text-based boundary logic.
	var remove []*html.Node
	visitNodes(doc, func(n *html.Node) {
		if n.Parent != nil && (profileBlockedNode(n) || n.Data == "form" || nodeAttr(n, "data-tariff-data") != "" || classToken(n, "payment-plan-card") || classToken(n, "intensive-weeks-section") || classToken(n, "tn-molecule")) {
			remove = append(remove, n)
		}
	})
	for _, n := range remove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
	h := profileContracts[c.Adapter]
	var text []string
	headingsList := [][]string{h.Audience, h.Requirements, h.Outcomes}
	if c.Adapter == "skillbox" {
		headingsList = append(headingsList, []string{"Составите резюме, которое притягивает оферы"}, []string{"Программа обучения"})
	}
	for _, headings := range headingsList {
		n := aiSemanticSection(doc, headings)
		if n != nil {
			text = append(text, nodeText(n))
		}
	}
	return aiNormalize(strings.Join(text, "\n"))
}

// Preserve subordinate headings inside a selected semantic block. Tilda often
// places its title in several wrapper nodes, separate from the actual content.
// A same-or-higher heading ends the block; duplicate roots fail closed.
func aiSemanticSection(doc *html.Node, headings []string) *html.Node {
	var found *html.Node
	var ambiguous bool
	visitNodes(doc, func(n *html.Node) {
		if n.Type != html.ElementNode || profileBlockedNode(n) {
			return
		}
		rank := func(tag string) int {
			switch tag {
			case "h2":
				return 2
			case "h3":
				return 3
			case "h4":
				return 4
			}
			return 0
		}
		level := rank(n.Data)
		if level == 0 {
			return
		}
		label := nodeText(n)
		match := false
		for _, h := range headings {
			if normalizeTitle(h) == normalizeTitle(label) {
				match = true
				break
			}
		}
		if !match {
			return
		}
		for root, depth := n.Parent, 0; root != nil && depth < 10; root, depth = root.Parent, depth+1 {
			if root.Data == "html" || root.Data == "body" || profileBlockedNode(root) {
				return
			}
			text := nodeText(root)
			if len(text) > 24000 {
				return
			}
			if at := strings.Index(text, label); at >= 0 {
				text = text[at:]
			}
			visitNodes(root, func(x *html.Node) {
				if x != n && rank(x.Data) > 0 && rank(x.Data) <= level {
					if at := strings.Index(text, nodeText(x)); at > 0 {
						text = text[:at]
					}
				}
			})
			if len(text) < len(label)+40 {
				continue
			}
			if found != nil {
				ambiguous = true
			}
			found = &html.Node{Type: html.ElementNode, Data: "section"}
			found.AppendChild(&html.Node{Type: html.TextNode, Data: text})
			return
		}
	})
	if ambiguous {
		return nil
	}
	return found
}

// Unlike course text, a selected tariff card may have a 'tariffs' ancestor.
func aiOfferText(n *html.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		switch x.Data {
		case "script", "style", "template", "noscript", "form", "input", "button", "nav", "footer":
			return
		}
		cls := strings.ToLower(nodeAttr(x, "class"))
		if profileHas(cls, "recommend", "related-course", "testimonial") {
			return
		}
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
			b.WriteByte(' ')
		}
		for child := x.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return aiNormalize(b.String())
}
