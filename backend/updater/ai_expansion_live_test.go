package updater

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// Explicit source preparation audit. No DB, model credentials, paid requests or
// production writes. Binding is derived from native source for this diagnostic;
// the real worker must additionally match the saved catalog mapping.
func TestAIExpansionLiveSources(t *testing.T) {
	if os.Getenv("AI_EXPANSION_SOURCE_CHECK") != "1" {
		t.Skip("opt-in official source-only audit")
	}
	samples := []struct{ adapter, url string }{
		{"skillbox", "https://skillbox.ru/course/profession-python"}, {"skillfactory", "https://skillfactory.ru/python-developer"},
		{"purpleschool", "https://purpleschool.ru/course/python-start"}, {"htmlacademy", "https://htmlacademy.ru/intensive/react"},
		{"skypro", "https://sky.pro/course/programming/python-web-course"}, {"otus", "https://otus.ru/lessons/python-basic"},
		{"hexlet", "https://ru.hexlet.io/programs/python"}, {"javarush", "https://javarush.com/prices"}, {"rsschool", "https://rs.school/courses/javascript"},
	}
	type result struct {
		Provider  string `json:"provider"`
		Code      string `json:"code"`
		Documents int    `json:"documents"`
		Bytes     int    `json:"bytes"`
	}
	var report []result
	for _, sample := range samples {
		if filter := os.Getenv("AI_SOURCE_PROVIDER"); filter != "" && filter != sample.adapter {
			continue
		}
		row := result{Provider: sample.adapter, Code: "source_unavailable"}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		client := NewClient()
		c, ok := aiCandidateURL(sample.url)
		if !ok {
			t.Fatal("invalid sample", sample)
		}
		raw, digest, e := fetch(ctx, client, candidateEndpoint(c))
		if e == nil {
			if dir := os.Getenv("AI_SOURCE_SNAPSHOT_DIR"); dir != "" {
				if err := os.WriteFile(dir+"/"+c.Adapter+".html", raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			remote, o, verifiedDigest, err := aiVerifyBoundSource(ctx, client, raw, digest, c)
			e = err
			if e != nil {
				row.Code = "native_" + rejectionCode(e, "source_rejected")
			}
			if e == nil {
				docs, err := aiBuildBoundDocuments(raw, c, remote, remote, o, aiBoundFixtureBinding(remote, o), verifiedDigest)
				e = err
				if e != nil {
					row.Code = "scope_rejected"
				}
				if e == nil {
					row.Code = "documents_ready"
					row.Documents = len(docs)
					for _, d := range docs {
						row.Bytes += len(d.Text)
					}
				}
			}
		}
		if e != nil && row.Code == "source_unavailable" {
			row.Code = rejectionCode(e, "source_unavailable")
		}
		cancel()
		client.CloseIdleConnections()
		report = append(report, row)
		b, _ := json.Marshal(row)
		t.Log(string(b))
	}
	if path := os.Getenv("AI_SOURCE_REPORT_PATH"); path != "" {
		data, _ := json.MarshalIndent(report, "", "  ")
		if e := os.WriteFile(path, data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	for _, r := range report {
		if r.Code != "documents_ready" {
			t.Errorf("%s: %s", r.Provider, r.Code)
		}
	}
}
