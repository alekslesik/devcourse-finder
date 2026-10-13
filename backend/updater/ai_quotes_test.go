package updater

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAIQuotesSelectedFromCompleteUnicodeDocument(t *testing.T) {
	text := strings.Repeat("Чтобы пройти курс, вам не нужны навыки или опыт — обучим с нуля. ", 100)
	segments := aiQuoteSegments(text)
	joined := []string{}
	for _, s := range segments {
		if utf8.RuneCountInString(s.Text) > 1000 {
			t.Fatal("oversized segment")
		}
		joined = append(joined, s.Text)
	}
	if strings.Join(joined, " ") != aiNormalize(text) {
		t.Fatal("document was cropped")
	}
	c, docs, x, _ := aiFixture()
	_ = c
	x.Claims[0].QuoteID = "s0"
	x.Claims[0].Quote = "hallucinated punctuation"
	if err := aiGroundQuotes(docs, &x); err != nil || x.Claims[0].Quote != aiNormalize(docs[0].Text) {
		t.Fatal("quote was not grounded", err)
	}
	x.Claims[0].QuoteID = "another-source"
	if err := aiGroundQuotes(docs, &x); err == nil {
		t.Fatal("unknown segment accepted")
	}
}
