package updater

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestVerifiedTariffsPublicationIdentityAndFailedConfirmation(t *testing.T) {
	for _, worker := range []string{"pages", ""} {
		t.Run(worker, func(t *testing.T) {
			db := postgresFixture(t)
			ctx := context.Background()
			b, c := skillboxFixture(t)
			now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
			r, o, e := collectSkillbox(b, c, now)
			if e != nil {
				t.Fatal(e)
			}
			// Extend only the synthetic observation lease to this test's wall-clock time.
			lease := time.Now().UTC().Add(24 * time.Hour)
			o.ValidUntil = &lease
			for i := range r.Offers {
				r.Offers[i].ValidUntil = &lease
			}
			r.CheckedAt = time.Now().UTC()
			config := productionConfig(t)
			config.Sources = nil
			config.Discovery = []Feed{{ID: c.FeedID, Adapter: c.Adapter, URL: "https://skillbox.ru/sitemap.xml"}}
			svc := Service{DB: db, Config: config, Worker: worker}
			code, n, e := svc.publishDiscovered(ctx, c, r, o, fingerprint(b))
			if e != nil || code != "verified" || n != 1 {
				t.Fatal(code, n, e)
			}
			courses, _ := db.Load(ctx)
			if len(courses) != 1 || len(courses[0].Offers) != 3 {
				t.Fatal(courses)
			}
			// Each offer has its own confirmation key. A failed read must interrupt all.
			for _, offer := range r.Offers {
				_, e = db.Pool.Exec(ctx, "INSERT INTO updater_candidates(source_id,fingerprint,confirmations,observed_at) VALUES($1,$2,1,now())", r.ID+":"+strings.TrimPrefix(offer.ID, r.ID+"-t"), fingerprint(b))
				if e != nil {
					t.Fatal(e)
				}
			}
			_, e = db.Pool.Exec(ctx, "INSERT INTO catalog_candidates(adapter,external_id,canonical_url,feed_id,state) VALUES($1,$2,$3,$4,'published')", c.Adapter, c.ExternalID, c.URL, c.FeedID)
			if e != nil {
				t.Fatal(e)
			}
			client := &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("unavailable")), Header: http.Header{}, Request: req}, nil
			})}
			stats, e := svc.processBatch(ctx, client, svc.publishDiscovered)
			if e != nil || stats.Failed != 1 {
				t.Fatal(stats, e)
			}
			if worker != "" {
				publisher := Service{DB: db, Config: config, Worker: "publisher"}
				if _, e = publisher.Run(ctx); e != nil {
					t.Fatal(e)
				}
			}
			var count int
			if e = db.Pool.QueryRow(ctx, "SELECT count(*) FROM updater_candidates").Scan(&count); e != nil || count != 0 {
				t.Fatal("failure left confirmation", count, e)
			}
			changed := o
			changed.VerifiedProductID++
			code, n, e = svc.publishDiscovered(ctx, c, r, changed, fingerprint(b))
			if e != nil || code != "protected_identity" || n != 0 {
				t.Fatal(code, n, e)
			}
		})
	}
}
