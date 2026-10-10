package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"devcourse-finder/catalog"
	"github.com/jackc/pgx/v5"
	"golang.org/x/net/html"
)

// A parent landing has at most a base program and two explicitly displayed plus
// programs. Failed tariffs carry only a fixed failure code, never source content.
type PracticumTariff struct {
	Slug         string     `json:"slug"`
	ProductID    string     `json:"product_id,omitempty"`
	ProfessionID string     `json:"profession_id,omitempty"`
	Price        int64      `json:"price,omitempty"`
	Enrollment   string     `json:"enrollment,omitempty"`
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
	FailureCode  string     `json:"failure_code,omitempty"`
}
type practicumTariffBinding struct {
	ProductID    string `json:"product_id"`
	ProfessionID string `json:"profession_id"`
	OfferID      string `json:"offer_id"`
}

func boundPracticumTariff(page []byte, c Candidate, slug string) bool {
	if !slugPattern.MatchString(slug) || !strings.HasSuffix(slug, "-plus") || practicumTariffPage(page, c, slug) != nil {
		return false
	}
	root, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return false
	}
	count := 0
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			attrs := map[string]string{}
			for _, a := range n.Attr {
				attrs[a.Key] = a.Val
			}
			if attrs["id"] == slug && strings.Contains(" "+attrs["class"]+" ", " common-flow-card__wrapper ") {
				links := 0
				var visit func(*html.Node)
				visit = func(x *html.Node) {
					if x.Type == html.ElementNode && x.Data == "a" {
						for _, a := range x.Attr {
							if a.Key == "href" && a.Val == practicumOrigin+"/profile/"+slug+"/" {
								links++
							}
						}
					}
					for ch := x.FirstChild; ch != nil; ch = ch.NextSibling {
						visit(ch)
					}
				}
				visit(n)
				if links == 1 {
					count++
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return count == 1
}
func practicumDisplayedTariffs(page []byte, c Candidate) ([]string, error) {
	// Read real DOM attributes only; escaped framework copies and the global price
	// map do not prove that a tariff belongs to this landing.
	root, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return nil, ErrSource
	}
	slugs := []string{}
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "div" {
			class := ""
			for _, a := range n.Attr {
				if a.Key == "class" {
					class = a.Val
				}
			}

			for _, a := range n.Attr {
				if strings.Contains(" "+class+" ", " common-flow-card__wrapper ") && a.Key == "id" && a.Val != c.ExternalID && !seen[a.Val] && boundPracticumTariff(page, c, a.Val) {
					seen[a.Val] = true
					slugs = append(slugs, a.Val)
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	if len(slugs) > 2 {
		return nil, rejection("identity_mismatch")
	}
	return slugs, nil
}
func fetchPracticumGroup(ctx context.Context, client *http.Client, page []byte, digest string, c Candidate) (catalog.Course, Observation, string, error) {
	record, o, digest, err := fetchPracticum(ctx, client, page, digest, c)
	if err != nil {
		return record, o, digest, err
	}
	slugs, err := practicumDisplayedTariffs(page, c)
	if err != nil {
		return catalog.Course{}, Observation{}, digest, err
	}
	if len(slugs) == 0 {
		return record, o, digest, nil
	}
	o.PracticumTariffs = []PracticumTariff{{Slug: c.ExternalID, ProductID: o.ProductID, ProfessionID: o.ProfessionID, Price: *o.Price, Enrollment: o.Enrollment, ValidUntil: o.ValidUntil}}
	now := record.CheckedAt
	lease := now.Add(26 * time.Hour)
	if o.ValidUntil == nil || lease.Before(*o.ValidUntil) {
		o.ValidUntil = &lease
		record.Offers[0].ValidUntil = &lease
	}
	o.PracticumTariffs[0].ValidUntil = o.ValidUntil
	for _, slug := range slugs {
		t := PracticumTariff{Slug: slug}
		bodies := make([][]byte, 3)
		var readErr error
		for i, u := range []string{practicumProfessionURL(slug), practicumPricesURL(slug), practicumSquadsURL(slug)} {
			var d string
			bodies[i], d, readErr = fetch(ctx, client, u)
			digest = fingerprint([]byte(digest + ":" + d))
			if readErr != nil {
				break
			}
		}
		if readErr == nil {
			r, obs, e := collectPracticumTariff(page, bodies[0], bodies[1], bodies[2], c, slug, now)
			readErr = e
			if e == nil && r.Language != record.Language {
				readErr = rejection("identity_mismatch")
			}
			if readErr == nil {
				if obs.ValidUntil == nil || lease.Before(*obs.ValidUntil) {
					obs.ValidUntil = &lease
					r.Offers[0].ValidUntil = &lease
				}
				t.ProductID, t.ProfessionID, t.Price, t.Enrollment, t.ValidUntil = obs.ProductID, obs.ProfessionID, *obs.Price, obs.Enrollment, obs.ValidUntil
				offer := r.Offers[0]
				offer.ID = record.ID + "-" + slug
				record.Offers = append(record.Offers, offer)
			}
		}
		if readErr != nil {
			t.FailureCode = rejectionCode(readErr, "source_unavailable")
		}
		o.PracticumTariffs = append(o.PracticumTariffs, t)
	}
	return record, o, digest, nil
}
func validPracticumGroup(e QueuedObservation) bool {
	o := e.Observation
	if e.Kind != "discovered" || e.Candidate.Adapter != "yandex" || len(o.PracticumTariffs) < 2 || len(o.PracticumTariffs) > 3 {
		return false
	}
	seen := map[string]bool{}
	products := map[string]bool{}
	professions := map[string]bool{}
	i := 0
	for j, t := range o.PracticumTariffs {
		if !slugPattern.MatchString(t.Slug) || seen[t.Slug] || j == 0 && t.Slug != e.Candidate.ExternalID || j > 0 && !strings.HasSuffix(t.Slug, "-plus") {
			return false
		}
		seen[t.Slug] = true
		if t.FailureCode != "" {
			if j == 0 || !failureCodeAllowed(t.FailureCode) || t.ProductID != "" || t.ProfessionID != "" || t.Price != 0 || t.Enrollment != "" || t.ValidUntil != nil {
				return false
			}
			continue
		}
		if !productUUID.MatchString(t.ProductID) || !productUUID.MatchString(t.ProfessionID) || products[t.ProductID] || professions[t.ProfessionID] || t.Price <= 0 || (t.Enrollment != "open" && t.Enrollment != "closed") || i >= len(e.Record.Offers) {
			return false
		}
		products[t.ProductID] = true
		professions[t.ProfessionID] = true
		r := e.Record.Offers[i]
		id := e.Record.ID + "-" + t.Slug
		if j == 0 {
			id = e.Record.ID + "-course"
			if o.ProductID != t.ProductID || o.ProfessionID != t.ProfessionID || o.Price == nil || *o.Price != t.Price || o.Enrollment != t.Enrollment {
				return false
			}
		}
		if r.ID != id || r.URL != e.Candidate.URL || r.Price == nil || *r.Price != t.Price || r.PriceKind != "exact" || r.Free || r.Enrollment != t.Enrollment || r.Schedule != "scheduled" || r.Mentor || r.Review || r.SupportKnown == nil || *r.SupportKnown {
			return false
		}
		if (r.ValidUntil == nil) != (t.ValidUntil == nil) || r.ValidUntil != nil && !r.ValidUntil.Equal(*t.ValidUntil) {
			return false
		}
		if t.Enrollment == "open" && (t.ValidUntil == nil || !t.ValidUntil.After(e.ObservedAt)) {
			return false
		}
		i++
	}
	return i == len(e.Record.Offers)
}
func mergePracticumGroup(ctx context.Context, tx pgx.Tx, old []catalog.Course, c Candidate, r catalog.Course, o Observation, now time.Time, code *string) ([]catalog.Course, error) {
	// Base identity migration uses the existing guarded contract, including curated
	// IDs. A failed/protected base cannot silently establish a new parent identity.
	base := r
	base.Offers = append([]catalog.Offer(nil), r.Offers[:1]...)
	baseObs := o
	baseObs.PracticumTariffs = nil
	baseObs.ValidUntil = nil
	updated, err := mergeDiscovered(ctx, tx, old, c, base, baseObs, now, code)
	if err != nil || *code == "protected_identity" || *code == "protected_course" || *code == "protected_offer" {
		if err == nil {
			_, err = tx.Exec(ctx, `DELETE FROM updater_candidates WHERE EXISTS(SELECT 1 FROM catalog_identities i WHERE i.adapter='yandex' AND i.external_id=$1 AND starts_with(updater_candidates.source_id,i.course_id||':'))`, c.ExternalID)
		}
		return updated, err
	}
	var courseID, baseOfferID string
	var raw []byte
	if err = tx.QueryRow(ctx, "SELECT course_id,offer_id,practicum_tariffs FROM catalog_identities WHERE adapter='yandex' AND external_id=$1", c.ExternalID).Scan(&courseID, &baseOfferID, &raw); errors.Is(err, pgx.ErrNoRows) {
		return updated, nil
	} else if err != nil {
		return updated, err
	}
	bindings := map[string]practicumTariffBinding{}
	if raw != nil && json.Unmarshal(raw, &bindings) != nil {
		return nil, ErrSource
	}
	var current catalog.Course
	found := false
	for _, x := range old {
		if x.ID == courseID {
			current = x
			found = true
		}
	}
	if len(updated) > 0 {
		current = updated[0]
		found = true
	}
	if !found {
		return nil, nil
	}
	current.Offers = append([]catalog.Offer(nil), current.Offers...)
	if len(updated) > 0 {
		for i := range current.Offers {
			if current.Offers[i].ID == baseOfferID {
				current.Offers[i].ValidUntil = o.ValidUntil
			}
		}
	}
	changed := len(updated) > 0
	pending := *code == "pending_confirmation"
	index := 1
	for _, t := range o.PracticumTariffs[1:] {
		key := courseID + ":" + t.Slug
		b, exists := bindings[t.Slug]
		if t.FailureCode != "" {
			*code = "partial_tariffs"
			slog.Warn("Practicum tariff retained", "slug", t.Slug, "code", t.FailureCode)
			if _, err = tx.Exec(ctx, "DELETE FROM updater_candidates WHERE source_id=$1", key); err != nil {
				return nil, err
			}
			continue
		}
		incoming := r.Offers[index]
		index++
		if t.ValidUntil != nil && !time.Now().Before(*t.ValidUntil) {
			*code = "partial_tariffs"
			if _, err = tx.Exec(ctx, "DELETE FROM updater_candidates WHERE source_id=$1", key); err != nil {
				return nil, err
			}
			continue
		}
		if exists && (b.ProductID != t.ProductID || b.ProfessionID != t.ProfessionID) {
			if _, err = tx.Exec(ctx, "DELETE FROM updater_candidates WHERE source_id=$1", key); err != nil {
				return nil, err
			}
			*code = "partial_tariffs"
			slog.Warn("Practicum tariff retained", "slug", t.Slug, "code", "protected_identity")
			continue
		}
		offerID := courseID + "-" + t.Slug
		if exists {
			offerID = b.OfferID
		}
		pos := -1
		for i, x := range current.Offers {
			if x.ID == offerID {
				pos = i
			}
		}
		if exists && (pos < 0 || current.Offers[pos].URL != c.URL) {
			*code = "partial_tariffs"
			slog.Warn("Practicum tariff retained", "slug", t.Slug, "code", "protected_offer")
			continue
		}
		if !exists && pos >= 0 {
			*code = "partial_tariffs"
			slog.Warn("Practicum tariff retained", "slug", t.Slug, "code", "protected_offer")
			continue
		}
		if !exists && t.Enrollment != "open" {
			continue
		}
		incoming.ID = offerID
		incoming.PriceCheckedAt = now
		if !exists {
			collision := false
			for _, x := range old {
				for _, offer := range x.Offers {
					if offer.ID == offerID {
						collision = true
					}
				}
			}
			if collision {
				*code = "partial_tariffs"
				slog.Warn("Practicum tariff retained", "slug", t.Slug, "code", "protected_offer")
				continue
			}
			current.Offers = append(current.Offers, incoming)
			bindings[t.Slug] = practicumTariffBinding{t.ProductID, t.ProfessionID, offerID}
			changed = true
			continue
		}
		template := current
		template.Offers = []catalog.Offer{incoming}
		local := "verified"
		obs := Observation{Price: &t.Price, Enrollment: t.Enrollment, Schedule: "scheduled"}
		merged, e := merge(ctx, tx, []catalog.Course{current}, template, key, obs, now, &local)
		if e != nil {
			return nil, e
		}
		if local == "pending_confirmation" {
			pending = true
			continue
		}
		if local != "verified" {
			*code = local
			continue
		}
		if len(merged) > 0 {
			current = merged[0]
			for i := range current.Offers {
				if current.Offers[i].ID == offerID {
					current.Offers[i].ValidUntil = t.ValidUntil
				}
			}
			changed = true
		}
	}
	if pending && *code == "verified" {
		*code = "pending_confirmation"
	}
	if !changed {
		return nil, nil
	}
	raw, err = json.Marshal(bindings)
	if err != nil {
		return nil, err
	}
	if _, err = tx.Exec(ctx, "UPDATE catalog_identities SET practicum_tariffs=$2::jsonb WHERE adapter='yandex' AND external_id=$1", c.ExternalID, raw); err != nil {
		return nil, err
	}
	return []catalog.Course{current}, nil
}
