package updater

import (
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"devcourse-finder/store"
	"golang.org/x/net/html"
)

type aiNetologyBinding struct {
	OfferID  string
	FamilyID int64
}

func aiFetchNetologyDocuments(ctx context.Context, db *store.DB, course catalog.Course, c Candidate, raw []byte, digest string) (Candidate, []AIDocument, error) {
	if db == nil {
		return c, nil, ErrAI
	}
	var binding aiNetologyBinding
	err := db.Pool.QueryRow(ctx, `SELECT offer_id,netology_family_id FROM catalog_identities WHERE adapter='netology' AND external_id=$1 AND canonical_url=$2 AND course_id=$3 AND netology_family_id>0`, c.ExternalID, c.URL, course.ID).Scan(&binding.OfferID, &binding.FamilyID)
	if err != nil {
		return c, nil, ErrAI
	}
	docs, err := aiNetologyDocuments(raw, c, course, binding, digest, time.Now().UTC())
	return c, docs, err
}

// Reuse the native product/payment identity contract. None of its price or
// enrollment observations are enqueued here; only scoped public text is output.
func aiNetologyDocuments(raw []byte, c Candidate, current catalog.Course, binding aiNetologyBinding, digest string, at time.Time) ([]AIDocument, error) {
	verified, observation, err := collectNetology(raw, c, at)
	if err != nil || binding.FamilyID <= 0 || observation.NetologyFamilyID != binding.FamilyID || current.Source != c.URL || verified.Language != current.Language || normalizeTitle(verified.Title) != normalizeTitle(current.Title) {
		return nil, ErrAI
	}
	var boundOffer *catalog.Offer
	for i := range current.Offers {
		o := &current.Offers[i]
		if o.ID == binding.OfferID && o.URL == c.URL && normalizeTitle(o.Name) == normalizeTitle(verified.Offers[0].Name) {
			boundOffer = o
		}
	}
	if boundOffer == nil {
		return nil, ErrAI
	}
	doc := learningDocument(raw)
	var page netologyPage
	var payload []byte
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "script" && nodeAttr(n, "id") == "__NEXT_DATA__" && n.FirstChild != nil {
			payload = []byte(n.FirstChild.Data)
		}
	})
	if decodeJSON(payload, &page) != nil {
		return nil, ErrAI
	}
	var plans []netologyPlan
	for key, content := range page.Props.PageProps.State.Landing.Content {
		if strings.HasPrefix(key, "plansNew_") {
			var plan netologyPlan
			if decodeJSON(content, &plan) != nil {
				return nil, ErrAI
			}
			plans = append(plans, plan)
		}
	}
	var docs []AIDocument
	if scope := netologyLearningScope(doc, plans, c, boundOffer.Name); scope != nil {
		text := nodeText(scope)
		if text != "" {
			docs = append(docs, AIDocument{ID: "offer:" + boundOffer.ID, Source: c.URL, Scope: "offer", OfferID: boundOffer.ID, Text: text, Digest: digest})
		}
	}
	// Every plan card, including other programs' cards, is removed from the
	// course-level document. An ambiguous root fails closed rather than leaking it.
	var titles []*html.Node
	visitNodes(doc, func(n *html.Node) {
		if n.Data == "p" && cardField.MatchString(nodeAttr(n, "name")) {
			titles = append(titles, n)
		}
	})
	roots := map[*html.Node]bool{}
	for _, title := range titles {
		root := aiNetologyCardRoot(title)
		if root == nil {
			return nil, ErrAI
		}
		roots[root] = true
	}
	for root := range roots {
		if root.Parent != nil {
			root.Parent.RemoveChild(root)
		}
	}
	var render strings.Builder
	if html.Render(&render, doc) != nil {
		return nil, ErrAI
	}
	text := aiPublicText(render.String())
	if len(text) < 200 {
		return nil, ErrAI
	}
	docs = append(docs, AIDocument{ID: "course", Source: c.URL, Scope: "course", Text: text, Digest: digest})
	for i := range docs {
		docs[i].BindingDigest = aiNetologyBindingDigest(current.ID, binding)
	}
	if !aiDocumentsValid(c, docs) {
		return nil, ErrAI
	}
	return docs, nil
}

func aiNetologyCardRoot(title *html.Node) *html.Node {
	for parent := title.Parent; parent != nil; parent = parent.Parent {
		titles, payments, features := 0, 0, 0
		visitNodes(parent, func(n *html.Node) {
			if n.Data == "p" && cardField.MatchString(nodeAttr(n, "name")) {
				titles++
			}
			if n.Data == "p" && strings.Contains(nodeAttr(n, "name"), ".features[") {
				features++
			}
			if classPrefix(n, "styles_prices__") {
				payments++
			}
		})
		if titles > 1 {
			return nil
		}
		if titles == 1 && payments == 1 && features > 0 {
			return parent
		}
	}
	return nil
}

func aiNetologyBindingDigest(courseID string, binding aiNetologyBinding) string {
	return fingerprint([]byte(fmt.Sprintf("%s\x00%s\x00%d", courseID, binding.OfferID, binding.FamilyID)))
}

func aiEventSourceIdentityValid(ctx context.Context, tx pgx.Tx, e AIEvent) (bool, error) {
	if e.Candidate.Adapter == "codebasics" {
		return aiCodeBasicsEventIdentityValid(ctx, tx, e)
	}
	if e.Candidate.Adapter != "netology" {
		return true, nil
	}
	var binding aiNetologyBinding
	err := tx.QueryRow(ctx, `SELECT offer_id,netology_family_id FROM catalog_identities WHERE adapter='netology' AND external_id=$1 AND canonical_url=$2 AND course_id=$3 AND netology_family_id>0`, e.Candidate.ExternalID, e.Candidate.URL, e.CourseID).Scan(&binding.OfferID, &binding.FamilyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	digest := aiNetologyBindingDigest(e.CourseID, binding)
	for _, doc := range e.Documents {
		if doc.BindingDigest != digest {
			return false, nil
		}
	}
	return true, nil
}
