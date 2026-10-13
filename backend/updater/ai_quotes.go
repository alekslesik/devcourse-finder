package updater

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

type AIQuoteSegment struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// Complete document segmentation, not truncation. The model selects an ID; its
// verbatim evidence is copied by our code so punctuation/Unicode cannot be invented.
func aiQuoteSegments(text string) []AIQuoteSegment {
	text = aiNormalize(text)
	out := []AIQuoteSegment{}
	for text != "" {
		limit := len(text)
		if utf8.RuneCountInString(text) > 1000 {
			runes := 0
			for i := range text {
				if runes == 1000 {
					limit = i
					break
				}
				runes++
			}
			// Prefer a sentence boundary near the end, then a word boundary.
			boundary := 0
			for i, r := range text[:limit] {
				if i > limit/2 && (r == '.' || r == '?' || r == '!') {
					boundary = i + len(string(r))
				}
			}
			if boundary == 0 {
				for i, r := range text[:limit] {
					if unicode.IsSpace(r) {
						boundary = i
					}
				}
			}
			if boundary > 0 {
				limit = boundary
			}
		}
		chunk := strings.TrimSpace(text[:limit])
		if chunk != "" {
			out = append(out, AIQuoteSegment{ID: "s" + strconv.Itoa(len(out)), Text: chunk})
		}
		text = strings.TrimSpace(text[limit:])
	}
	return out
}
func aiPromptDocuments(docs []AIDocument) []map[string]any {
	out := []map[string]any{}
	for _, d := range docs {
		out = append(out, map[string]any{"id": d.ID, "source": d.Source, "scope": d.Scope, "offer_id": d.OfferID, "segments": aiQuoteSegments(d.Text)})
	}
	return out
}
func aiGroundQuotes(docs []AIDocument, x *AIExtraction) error {
	segments := map[string]map[string]string{}
	for _, d := range docs {
		segments[d.ID] = map[string]string{}
		for _, s := range aiQuoteSegments(d.Text) {
			segments[d.ID][s.ID] = s.Text
		}
	}
	for i := range x.Claims {
		claim := &x.Claims[i]
		if claim.QuoteID == "" {
			continue
		} // Legacy literal quoted fixtures remain supported.
		quote, ok := segments[claim.DocumentID][claim.QuoteID]
		if !ok {
			return ErrAI
		}
		claim.Quote = quote
	}
	return nil
}
