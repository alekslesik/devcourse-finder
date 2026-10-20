package updater

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"devcourse-finder/catalog"
)

func TestAIExplicitSelectionRejectsBroadMalformedAndDuplicateInputs(t *testing.T) {
	for _, raw := range []string{"", "*", "netology-%", "https://netology.ru/programs/go", "netology-go,", "netology-go,netology-go", "netology-go, hexlet-java", "a,b,c,d,e,f,g,h,i"} {
		if _, err := ParseAICourseIDs(raw); err == nil {
			t.Fatal("unsafe selection accepted", raw)
		}
	}
	ids, err := ParseAICourseIDs("netology-go,hexlet-java")
	if err != nil || !reflect.DeepEqual(ids, []string{"netology-go", "hexlet-java"}) {
		t.Fatal(ids, err)
	}
}

func TestAISourceCheckReportsPartialTariffGapWithoutWrites(t *testing.T) {
	db := postgresFixture(t)
	raw, _, record := hexletPublicFixture(t, "java-basics-free")
	ctx := context.Background()
	if err := db.Import(ctx, catalog.Dataset{Domains: []string{"ru.hexlet.io"}, Courses: []catalog.Course{record}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	before, _ := db.Load(ctx)
	requests := 0
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.URL.String() != record.Source {
			t.Fatal("unexpected source", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(raw))}, nil
	})}
	var out bytes.Buffer
	if err := AISourceCheck(ctx, db, client, record.ID+",missing", &out); err == nil || requests != 0 {
		t.Fatal("selection was not prevalidated", requests, err)
	}
	if err := AISourceCheck(ctx, db, client, record.ID, &out); err != nil {
		t.Fatal(err)
	}
	var report struct {
		ModelRequests, Writes int
		Results               []aiSourceCheckResult
	}
	if err := json.Unmarshal(out.Bytes(), &report); err != nil || len(report.Results) != 1 || report.Results[0].Code != "partial_course_only_tariff_unverified" || report.Results[0].OfferDocuments != 0 {
		t.Fatal(out.String(), err)
	}
	after, _ := db.Load(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("source check changed catalog")
	}
	for _, table := range []string{"catalog_ai_cache", "catalog_ai_daily_budget", "catalog_ai_sources", "catalog_ai_observations", "catalog_ai_runs"} {
		var count int
		if err := db.Pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count); err != nil || count != 0 {
			t.Fatal(table, count, err)
		}
	}
	client.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("private upstream detail"))}, nil
	})
	out.Reset()
	if err := AISourceCheck(ctx, db, client, record.ID, &out); err == nil || strings.Contains(out.String(), "private upstream") {
		t.Fatal(out.String(), err)
	}
}
