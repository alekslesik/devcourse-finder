package updater

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestQueueBalancesProvidersAndRefreshWithoutStarvation(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	config.Discovery = []Feed{{ID: "otus", Adapter: "otus", URL: "https://otus.ru/sitemap.xml"}, {ID: "stepik", Adapter: "stepik", URL: "https://stepik.org/sitemap.xml"}}
	service := Service{DB: db, Config: config}
	for _, adapter := range []string{"otus", "stepik"} {
		for i := 0; i < 20; i++ {
			for _, state := range []string{"pending", "published"} {
				id := fmt.Sprintf("%s-%d", state, i)
				_, err := db.Pool.Exec(ctx, "INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state) VALUES($1,$2,$3,$1,$4)", adapter, id, "https://"+providerHosts[adapter]+"/"+id, state)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	work, err := service.queueWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, w := range work {
		counts[w.Adapter+"-"+w.State]++
	}
	if len(work) != 24 || len(counts) != 4 {
		t.Fatal("provider/lane starvation", counts)
	}
	for _, n := range counts {
		if n != 6 {
			t.Fatal("lane limit exceeded", counts)
		}
	}
	// Claimed but interrupted work becomes eligible after its lease expires.
	w := work[0]
	db.Pool.Exec(ctx, "UPDATE catalog_candidates SET next_attempt_at=now()+interval '15 minutes' WHERE adapter=$1 AND external_id=$2", w.Adapter, w.ExternalID)
	again, err := service.queueWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, x := range again {
		if x.Adapter == w.Adapter && x.ExternalID == w.ExternalID {
			t.Fatal("active lease selected")
		}
	}
	db.Pool.Exec(ctx, "UPDATE catalog_candidates SET next_attempt_at=now()-interval '1 second' WHERE adapter=$1 AND external_id=$2", w.Adapter, w.ExternalID)
	again, err = service.queueWork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, x := range again {
		if x.Adapter == w.Adapter && x.ExternalID == w.ExternalID {
			found = true
		}
	}
	if !found {
		t.Fatal("expired lease did not resume")
	}
}
func TestBackoffIsBounded(t *testing.T) {
	for i := 0; i < 100; i++ {
		d := retryDelay(i)
		if d < 12*time.Hour || d > 72*time.Hour {
			t.Fatal("unbounded retry", d)
		}
	}
}
