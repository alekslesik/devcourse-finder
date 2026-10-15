package updater

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Opt-in, read-only semantic benchmark: 2 captured official samples, at most
// 4 paid requests, 5m total. No production database, imports or publication.
func TestOpenAIRecordedSourceBenchmark(t *testing.T) {
	if os.Getenv("OPENAI_LIVE_CHECK") != "1" {
		t.Skip("opt-in bounded OpenAI benchmark")
	}
	config, err := LoadAIConfig()
	if err != nil {
		t.Fatal("OpenAI secret configuration is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	model := &OpenAIModel{Key: config.Key, Model: config.Model, VerifyModel: config.VerifyModel}
	type row struct {
		Provider    string    `json:"provider"`
		Source      string    `json:"source"`
		Mode        string    `json:"mode"`
		Model       string    `json:"model"`
		NativeFacts int       `json:"native_profile_facts"`
		AIFacts     int       `json:"ai_profile_facts"`
		Claims      []AIClaim `json:"verified_claims"`
		Usage       AIUsage   `json:"usage"`
	}
	report := []row{}
	for _, adapter := range []string{"stepik", "yandex"} {
		c := Candidate{Adapter: adapter, ExternalID: "54403", URL: "https://stepik.org/course/54403/promo"}
		path := "testdata/profiles/stepik-go.json"
		if adapter == "yandex" {
			c.ExternalID = "backend-developer"
			c.URL = "https://practicum.yandex.ru/backend-developer"
			path = "testdata/profiles/yandex.html"
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var text string
		var baseline int
		if adapter == "stepik" {
			var course stepikCourse
			if json.Unmarshal(raw, &course) != nil {
				t.Fatal("invalid recorded fixture")
			}
			text = "Название: " + course.Title + "\nОписание: " + aiPublicText(course.Summary) + "\nТребования: " + aiPublicText(course.Requirements) + "\nАудитория: " + aiPublicText(course.TargetAudience)
			baseline = len(stepikProfile(raw, course, c, time.Now()).Evidence())
		} else {
			text = aiPublicText(string(raw))
			baseline = len(profileFromSections(raw, c, "python", time.Now(), profileContracts[c.Adapter]).Evidence())
		}
		source := c.URL
		if adapter == "stepik" {
			source = candidateEndpoint(c)
		}
		docs := []AIDocument{{ID: "course", Source: source, Scope: "course", Text: text, Digest: fingerprint(raw)}}
		x, usage, err := model.Extract(ctx, c, docs)
		if err != nil {
			t.Fatal("OpenAI extraction failed", err)
		}
		x, err = aiIsolateClaims(c, docs, x, time.Now())
		if err != nil {
			t.Fatalf("extraction shape or source quote rejected: %+v", x)
		}
		v, more, err := model.Verify(ctx, c, docs, x)
		if err != nil {
			t.Fatal("OpenAI verification failed", err)
		}
		facts, err := aiCompile(c, docs, x, v, time.Now())
		if err != nil {
			t.Fatal("independently verified output rejected")
		}
		usage.Prompt += more.Prompt
		usage.Completion += more.Completion
		usage.Total += more.Total
		accepted := []AIClaim{}
		for _, i := range v.Verified {
			accepted = append(accepted, x.Claims[i])
		}
		report = append(report, row{Provider: adapter, Source: source, Mode: "captured_official_fragment_not_live_publication", Model: config.Model + "+" + config.VerifyModel, NativeFacts: baseline, AIFacts: len(facts.Profile.Evidence()), Claims: accepted, Usage: usage})
		t.Log(adapter, "native_facts", baseline, "AI_facts", len(facts.Profile.Evidence()), "tokens", usage.Total)
	}
	if path := os.Getenv("OPENAI_BENCHMARK_REPORT_PATH"); path != "" {
		raw, _ := json.MarshalIndent(report, "", "  ")
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
