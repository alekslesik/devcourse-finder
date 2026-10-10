package updater

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestPracticumLiveVerificationAndPublication(t *testing.T) {
	if os.Getenv("PRACTICUM_LIVE_CHECK") != "1" {
		t.Skip("set PRACTICUM_LIVE_CHECK=1 with a disposable TEST_DATABASE_URL for live verification")
	}
	db := postgresFixture(t)
	c := Candidate{Adapter: "yandex", ExternalID: "backend-developer", URL: practicumOrigin + "/backend-developer", FeedID: "yandex-sitemap"}
	ctx, cancel := context.WithTimeout(context.Background(), 350*time.Second)
	defer cancel()
	client := NewClient()
	page, pageDigest, err := fetch(ctx, client, candidateEndpoint(c))
	if err != nil {
		t.Fatal(err)
	}
	record, o, digest, err := fetchPracticumGroup(ctx, client, page, pageDigest, c)
	if err != nil {
		t.Fatal(err)
	}
	config := productionConfig(t)
	config.Sources = nil
	config.Discovery = []Feed{{ID: c.FeedID, Adapter: c.Adapter, URL: practicumOrigin + "/lang-static/sitemap/"}}
	s := Service{DB: db, Config: config, Worker: "publisher"}
	if err = s.enqueue(ctx, QueuedObservation{Kind: "discovered", Candidate: c, Record: record, Observation: o, ObservedAt: record.CheckedAt, Digest: digest}); err != nil {
		t.Fatal(err)
	}
	result, err := s.Publish(ctx)
	if err != nil || result.Published != 1 {
		t.Fatal(result, err)
	}
	t.Logf("Verified grouped offers: %d", len(record.Offers))
	t.Logf("Real anonymous official evidence published in disposable DB: %s; full price=%d RUB; enrollment=%s; free=false", record.Title, *o.Price/100, o.Enrollment)
}
