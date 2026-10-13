package updater

import (
	"os"
	"strings"
	"testing"
)

func TestAIResourceBoundedHTMLDocument(t *testing.T) {
	// A large embedded application payload must never become model input; the
	// complete permitted public text remains present without truncation.
	public := strings.Repeat("Курс подходит начинающим без опыта. Вы научитесь создавать собственные проекты. ", 300)
	raw := `<html><head><script>` + strings.Repeat("application-state ", 200000) + `</script></head><body><main>` + public + `</main></body></html>`
	text := aiPublicText(raw)
	if text != aiNormalize(public) {
		t.Fatal("public text changed or script leaked")
	}
	c, docs, _, _ := aiFixture()
	docs = docs[:1]
	docs[0].Text = text
	if !aiDocumentsValid(c, docs) || len(aiQuoteSegments(text)) < 2 {
		t.Fatal("bounded document rejected")
	}
	if raw, err := os.ReadFile("/proc/self/status"); err == nil {
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.HasPrefix(line, "VmHWM:") {
				t.Log("process_peak_rss", line)
			}
		}
	}
}
