package updater

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestNetologyLiveVerification(t *testing.T) {
	if os.Getenv("NETOLOGY_LIVE_CHECK") != "1" {
		t.Skip("set NETOLOGY_LIVE_CHECK=1 for bounded official reads")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	for _, slug := range []string{"java-developer", "go"} {
		c := Candidate{Adapter: "netology", ExternalID: slug, URL: "https://netology.ru/programs/" + slug, FeedID: "netology-catalog"}
		data, _, err := fetch(ctx, NewClient(), c.URL)
		if err != nil {
			t.Fatal(slug, err)
		}
		record, o, err := collectNetology(data, c, time.Now().UTC())
		if err != nil {
			t.Fatal(slug, err)
		}
		t.Logf("%s: full price %d RUB, enrollment %s", record.Title, *o.Price/100, o.Enrollment)
	}
}
