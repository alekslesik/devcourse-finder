package updater

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestVerifiedNodeDirectionRepairAuditFinancialAndIdentityGuards(t *testing.T) {
	for _, scenario := range []string{"valid", "foreign-page", "missing-outcome", "curated-direction", "missing-binding"} {
		t.Run(scenario, func(t *testing.T) {
			db := postgresFixture(t)
			ctx := context.Background()
			page, _, _ := practicumData(t, "backend-developer")
			page = []byte(strings.ReplaceAll(string(page), "backend-developer", "backend-nodejs"))
			page = append(page, []byte("<p>"+nodeOutcomeQuote+". Научитесь работать с MongoDB и PostgreSQL, создавать API и веб-сервисы на Express.</p>")...)
			c := Candidate{Adapter: "yandex", ExternalID: "backend-nodejs", URL: practicumOrigin + "/backend-nodejs"}
			price := int64(6350000)
			record, err := normalizedRecord(c, "Бэкенд на Node.js для фронтенд-разработчиков", "Серверная разработка на Node.js", "Яндекс Практикум", Observation{Price: &price, Enrollment: "open", Schedule: "scheduled"}, time.Now().UTC())
			if err != nil {
				t.Fatal(err)
			}
			record.Direction = "frontend"
			if scenario == "curated-direction" {
				record.Profile = &catalog.CourseProfile{Direction: &catalog.ProfileValue{Value: "frontend", Evidence: learningProof(page, c.URL, "curated", time.Now().UTC())}}
			}
			if err = db.Import(ctx, catalog.Dataset{Domains: []string{"practicum.yandex.ru"}, Courses: []catalog.Course{record}}, "operator", false); err != nil {
				t.Fatal(err)
			}
			before, err := db.Load(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = db.Pool.Exec(ctx, `INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id) VALUES('yandex',$1,$2,$3,$4)`, c.ExternalID, c.URL, record.ID, record.Offers[0].ID); err != nil {
				t.Fatal(err)
			}
			requests := 0
			client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
				requests++
				if r.URL.String() != candidateEndpoint(c) {
					t.Fatal(r.URL)
				}
				body := string(page)
				if scenario == "foreign-page" {
					body = strings.ReplaceAll(body, "backend-nodejs", "other-course")
				}
				if scenario == "missing-outcome" {
					body = strings.ReplaceAll(body, nodeOutcomeQuote, "Нерелевантное описание")
				}
				response := io.NopCloser(strings.NewReader(body))
				return &http.Response{StatusCode: 200, Body: response}, nil
			})}
			// A missing saved identity must preserve every catalog field.
			if scenario == "missing-binding" {
				if _, err = db.Pool.Exec(ctx, `DELETE FROM catalog_identities`); err != nil {
					t.Fatal(err)
				}
			}
			var out bytes.Buffer
			err = RepairNodeDirection(ctx, db, client, &out)
			after, loadErr := db.Load(ctx)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			var audits int
			if e := db.Pool.QueryRow(ctx, `SELECT count(*) FROM imports WHERE operator='verified-node-direction-repair'`).Scan(&audits); e != nil {
				t.Fatal(e)
			}
			if scenario != "valid" {
				if err == nil || !reflect.DeepEqual(before, after) || audits != 0 {
					t.Fatal("unsafe repair", err, audits, after)
				}
				return
			}
			if err != nil || len(after) != 1 || after[0].Direction != "backend" || after[0].Profile == nil || after[0].Profile.Direction.Value != "backend" || audits != 1 {
				t.Fatal(out.String(), after, err, audits)
			}
			corrected := after[0]
			corrected.Direction = before[0].Direction
			corrected.Profile = before[0].Profile
			if !reflect.DeepEqual(corrected, before[0]) {
				t.Fatal("repair changed unrelated fields", corrected, before[0])
			}
			n := requests
			if err = RepairNodeDirection(ctx, db, client, &out); err != nil || requests != n {
				t.Fatal("repair was not idempotent", err, requests)
			}
			if e := db.Pool.QueryRow(ctx, `SELECT count(*) FROM imports WHERE operator='verified-node-direction-repair'`).Scan(&audits); e != nil || audits != 1 {
				t.Fatal(e, audits)
			}
		})
	}
}
