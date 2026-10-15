package updater

import (
	"testing"
	"time"
)

func TestAIRegistryRejectsUnsupportedAndNoncanonicalCandidates(t *testing.T) {
	for _, url := range []string{"https://stepik.org/course/42/promo?x=1", "https://stepik.org/course/42/promo/", "https://stepik.org.evil.example/course/42/promo", "https://user@stepik.org/course/42/promo", "http://stepik.org/course/42/promo", "https://skillbox.ru/course/profession-python"} {
		if _, ok := aiCandidateURL(url); ok {
			t.Fatal("unsupported/noncanonical source enabled", url)
		}
	}
	c, docs, _, _ := aiFixture()
	c.Adapter = "yandex"
	if aiDocumentsValid(c, docs) {
		t.Fatal("adapter did not match URL")
	}
	c, _, _, _ = aiFixture()
	c.ExternalID = "43"
	if aiDocumentsValid(c, docs) {
		t.Fatal("external ID did not match URL")
	}
}
func TestAIRegistrySourcesRemainValidAndBudgetScheduleUnchanged(t *testing.T) {
	for _, url := range []string{"https://stepik.org/course/42/promo", "https://practicum.yandex.ru/backend-developer", "https://netology.ru/programs/go", "https://code-basics.com/ru/languages/python"} {
		c, ok := aiCandidateURL(url)
		if !ok {
			t.Fatal(url)
		}
		docs := []AIDocument{{ID: "course", Scope: "course", Source: c.URL, Text: "Официальный текст программы обучения.", BindingDigest: fingerprint([]byte("binding")), Digest: fingerprint([]byte("source"))}}
		if !aiDocumentsValid(c, docs) {
			t.Fatal(c)
		}
	}
	if len(aiSourcePatterns()) != len(aiProviders) {
		t.Fatal("SQL registry diverged")
	}
	// Registering providers must not create paid initial runs or off-day schedules.
	offDay, _ := time.Parse(time.RFC3339, "2026-10-16T21:00:00+03:00")
	if aiScheduledRunDue(offDay, nil) {
		t.Fatal("registry changed weekly budget scheduling")
	}
}
