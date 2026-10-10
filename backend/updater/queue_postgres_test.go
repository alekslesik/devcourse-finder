package updater

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestQueueBalancesProvidersAndRefreshWithoutStarvation(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	config := productionConfig(t)
	config.Discovery = []Feed{{ID: "otus", Adapter: "otus", URL: "https://otus.ru/sitemap.xml"}, {ID: "stepik", Adapter: "stepik", URL: "https://stepik.org/sitemap.xml"}}
	service := Service{DB: db, Config: config}
	for _, adapter := range []string{"otus", "stepik"} {
		for i := 0; i < 150; i++ {
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
	if len(work) != 162 || len(counts) != 4 {
		t.Fatal("provider/lane starvation", counts)
	}
	for lane, n := range counts {
		expected := providerLaneLimit
		if lane == "stepik-pending" {
			expected = stepikNewLimit
		}
		if lane == "stepik-published" {
			expected = stepikRefreshLimit
		}
		if n != expected {
			t.Fatal("lane limit exceeded", counts)
		}
	}
	// Other providers and published refreshes must get a turn before Stepik's
	// large discovery backlog, even if the deadline truncates the batch.
	for _, lane := range []string{"otus-pending", "otus-published", "stepik-published"} {
		found := false
		for _, w := range work[:4] {
			if w.Adapter+"-"+w.State == lane {
				found = true
			}
		}
		if !found {
			t.Fatal("early provider/lane starvation", lane)
		}
	}
	// Full production provider coverage still fits in the global budget.
	for _, adapter := range []string{"yandex", "hexlet", "codebasics", "netology", "purpleschool", "rsschool", "htmlacademy", "skillbox", "skillfactory", "skypro", "javarush"} {
		service.Config.Discovery = append(service.Config.Discovery, Feed{ID: adapter, Adapter: adapter})
		for i := 0; i < 10; i++ {
			for _, state := range []string{"pending", "published"} {
				id := fmt.Sprintf("%s-%d", state, i)
				if _, err = db.Pool.Exec(ctx, "INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state) VALUES($1,$2,$3,$1,$4)", adapter, id, "https://"+providerHosts[adapter]+"/"+id, state); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
	all, err := service.queueWork(ctx)
	if err != nil || len(all) != 294 || len(all) > batchLimit {
		t.Fatal("global provider budget", len(all), err)
	}
	lanes := map[string]bool{}
	for _, w := range all[:26] {
		lanes[w.Adapter+"-"+w.State] = true
	}
	if len(lanes) != 26 {
		t.Fatal("provider missed first round", lanes)
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

func TestExpandedBatchStopsBeforeClaimingWhenBudgetIsLow(t *testing.T) {
	db := postgresFixture(t)
	config := productionConfig(t)
	config.Discovery = []Feed{{ID: "stepik", Adapter: "stepik", URL: "https://stepik.org/sitemap.xml"}}
	service := Service{DB: db, Config: config}
	_, err := db.Pool.Exec(context.Background(), `INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state) VALUES('stepik','54403','https://stepik.org/course/54403/promo','stepik','pending')`)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("detail fetched without sufficient budget")
		return nil, ErrSource
	})}
	stats, err := service.processBatch(ctx, client, func(context.Context, Candidate, catalog.Course, Observation, string) (string, int, error) {
		t.Fatal("unexpected publication")
		return "", 0, nil
	})
	if err != nil || stats.Attempted != 0 {
		t.Fatal(stats, err)
	}
	var unclaimed bool
	if err = db.Pool.QueryRow(ctx, `SELECT attempted_at IS NULL FROM catalog_candidates`).Scan(&unclaimed); err != nil || !unclaimed {
		t.Fatal("candidate claimed without budget", err)
	}
}
