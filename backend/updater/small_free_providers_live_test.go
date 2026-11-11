package updater

import (
	"context"
	"os"
	"testing"
	"time"
)

// Opt-in anonymous source preparation only: no production DB or model calls.
// Production AI still requires the saved identity at read and publication time.
func TestSmallFreeProvidersLiveOfficialSources(t *testing.T) {
	if os.Getenv("SMALL_FREE_SOURCE_CHECK") != "1" {
		t.Skip("opt-in public source-only acceptance")
	}
	client := NewClient()
	defer client.CloseIdleConnections()
	for _, adapter := range []string{"codechick", "learnpython"} {
		t.Run(adapter, func(t *testing.T) {
			_, c := smallFreeFixture(t, adapter)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			raw, digest, err := fetch(ctx, client, candidateEndpoint(c))
			if err != nil {
				t.Fatal(err)
			}
			r, o, err := collectCandidate(raw, c, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			docs, err := aiBuildBoundDocuments(raw, c, r, r, o, aiBoundFixtureBinding(r, o), digest)
			if err != nil || len(docs) != 1 || !aiDocumentsValid(c, docs) {
				t.Fatal(docs, err)
			}
			feed := Feed{ID: c.FeedID, Adapter: adapter, Kind: "catalog", URL: "https://codechick.io/"}
			if adapter == "learnpython" {
				feed.URL = "https://www.learnpython.org/ru/"
			}
			root, _, err := fetch(ctx, client, feed.URL)
			if err != nil {
				t.Fatal(err)
			}
			links, err := catalogLinks(root, feed)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, link := range links {
				if linked, ok := candidateURL(adapter, link, feed.ID); ok && linked.URL == c.URL {
					found = true
				}
			}
			if !found {
				t.Fatal("course missing from official discovery root")
			}
			t.Logf("%s: full free course verified, official discovery verified, scoped course bytes=%d; no teacher support or calendar duration claimed; model requests=0", r.Provider, len(docs[0].Text))
		})
	}
}
