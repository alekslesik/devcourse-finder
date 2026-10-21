package updater

import (
	"bytes"
	"golang.org/x/net/html"
	"os"
	"strings"
	"testing"
)

func TestAIHexletTariffsBindCTAThroughPopupToPayment(t *testing.T) {
	for _, slug := range []string{"java", "frontend"} {
		t.Run(slug, func(t *testing.T) {
			_, c, r := hexletPublicFixture(t, slug)
			raw, err := os.ReadFile("testdata/hexlet-public-" + slug + "-tariffs.html")
			if err != nil {
				t.Fatal(err)
			}
			docs, err := aiHexletPublic(raw, c, r, nil, fingerprint(raw))
			if err != nil || len(docs) != 4 || !aiHexletPublicBundle(docs, r.ID) {
				t.Fatal(docs, err)
			}
			for _, d := range docs[1:] {
				if strings.Contains(d.Text, "₽") || strings.Contains(d.Text, "рассроч") || strings.Contains(d.Text, "Все опции") {
					t.Fatal("billing/inheritance leaked", d)
				}
				if strings.Contains(d.OfferID, "-standard") && !strings.Contains(d.Text, "10 месяцев") {
					t.Fatal(d)
				}
				if strings.Contains(d.OfferID, "-premium") && (!strings.Contains(d.Text, "12 месяцев") || !strings.Contains(d.Text, "80 часов персональных")) {
					t.Fatal(d)
				}
				if !strings.Contains(d.OfferID, "-premium") && strings.Contains(d.Text, "80 часов") {
					t.Fatal("premium leaked", d)
				}
			}
			// Duplicating an otherwise valid card keeps course identity intact
			// but makes that single tariff ambiguous.
			doc := learningDocument(raw)
			var first *html.Node
			visitNodes(doc, func(n *html.Node) {
				if first == nil && classToken(n, "tn-molecule") && strings.HasPrefix(aiNormalize(nodeText(n)), "Стандартный ") && strings.Contains(nodeText(n), "10 месяцев") {
					first = n
				}
			})
			if first == nil {
				t.Fatal("fixture card missing")
			}
			var rendered bytes.Buffer
			if err := html.Render(&rendered, first); err != nil {
				t.Fatal(err)
			}
			duplicate := strings.Replace(string(raw), "</body>", rendered.String()+"</body>", 1)
			duplicated, err := aiHexletPublic([]byte(duplicate), c, r, nil, fingerprint([]byte(duplicate)))
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range duplicated {
				if strings.HasSuffix(d.OfferID, "-standard") {
					t.Fatal("duplicate card accepted")
				}
			}
			// A mismatched popup/checkout or copied duplicate card must not supply tier facts.
			bad := strings.Replace(string(raw), `data-tooltip-hook="#zeropopup_standard"`, `data-tooltip-hook="#other"`, 1)
			got, err := aiHexletPublic([]byte(bad), c, r, nil, fingerprint([]byte(bad)))
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range got {
				if strings.HasSuffix(d.OfferID, "-standard") {
					t.Fatal("unbound popup accepted")
				}
			}
		})
	}
}
func TestAIHexletFreeIdentityRequiresOwnLearningLink(t *testing.T) {
	for _, slug := range []string{"java-basics-free", "javascript-basics-free"} {
		raw, c, r := hexletPublicFixture(t, slug)
		docs, err := aiHexletPublic(raw, c, r, nil, fingerprint(raw))
		if err != nil || len(docs) != 1 {
			t.Fatal(docs, err)
		}
		bad := strings.ReplaceAll(string(raw), "/goto", "/other")
		if _, err := aiHexletPublic([]byte(bad), c, r, nil, fingerprint([]byte(bad))); err == nil {
			t.Fatal("foreign free enrollment")
		}
	}
}
