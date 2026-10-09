package updater

import "testing"

func TestWorkerOwnershipFiltersSourcesAndFeeds(t *testing.T) {
	c, err := LoadConfig("../../data/updater-sources.json", "../../data/real-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"api", "pages"} {
		filtered, err := c.ForWorker(role)
		if err != nil {
			t.Fatal(err)
		}
		for _, source := range filtered.Sources {
			if owner(sourceAdapter(source.Adapter)) != role {
				t.Fatal("wrong source owner")
			}
		}
		for _, feed := range filtered.Discovery {
			if owner(feed.Adapter) != role {
				t.Fatal("wrong feed owner")
			}
		}
		if role == "pages" && len(filtered.Sources) != 0 {
			t.Fatal("pages selected API curated records")
		}
	}
	if _, err := c.ForWorker("unexpected"); err == nil {
		t.Fatal("unknown worker accepted")
	}
	if got, err := c.ForWorker("publisher"); err != nil || len(got.Discovery) != len(c.Discovery) {
		t.Fatal("publisher lost approved sources", err)
	}
}
