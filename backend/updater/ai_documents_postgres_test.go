package updater

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func TestAIStepikDocumentIdentityAndPrivacy(t *testing.T) {
	c := Candidate{Adapter: "stepik", ExternalID: "54403", URL: "https://stepik.org/course/54403/promo"}
	raw := validStepik()
	raw["language"] = "ru"
	raw["summary"] = "Курс посвящён изучению Golang и созданию собственных программ с практическими заданиями."
	raw["requirements"] = "Нужны базовые знания программирования."
	raw["workload"] = "8–12 часов в неделю"
	course := catalog.Course{ID: "stepik-54403", Language: "go", Source: c.URL, Status: "published", Offers: []catalog.Offer{{ID: "stepik-54403-course", URL: c.URL}}}
	client := func(body []byte) *http.Client {
		return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.String() != candidateEndpoint(c) {
				t.Fatal(r.URL)
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})}
	}
	if _, docs, err := aiFetchDocuments(context.Background(), nil, client(stepikJSON(raw)), course); err != nil || len(docs) != 2 {
		t.Fatal(docs, err)
	}
	for _, field := range []string{"is_public", "is_active", "is_enabled"} {
		bad := map[string]any{}
		for k, v := range raw {
			bad[k] = v
		}
		bad[field] = false
		if _, _, err := aiFetchDocuments(context.Background(), nil, client(stepikJSON(bad)), course); err == nil {
			t.Fatal("private/inactive document accepted", field)
		}
	}
	raw["id"] = 999
	if _, _, err := aiFetchDocuments(context.Background(), nil, client(stepikJSON(raw)), course); err == nil {
		t.Fatal("wrong course accepted")
	}
}
func TestAIPracticumSeparatesPlusAndBaseDocuments(t *testing.T) {
	db := postgresFixture(t)
	page, _, _, c := sharedPracticum(t)
	page = []byte(strings.Replace(string(page), "</body>", `<div class="common-flow-card__wrapper" id="backend-developer"><p>Базовый тариф. Задания проверяет преподаватель.</p></div><section><h2>Кому подходит программа</h2><p>`+strings.Repeat("Программа рассчитана на начинающих без опыта. ", 8)+`</p></section></body>`, 1))
	page = []byte(strings.Replace(string(page), `id="python-developer-plus">`, `id="python-developer-plus"><p>PLUS_ONLY_MENTOR: персональный наставник.</p>`, 1))
	price := int64(10000000)
	at := time.Now().UTC()
	record, err := normalizedRecord(c, "Python-разработчик", "Обучение Python с практикой", "Практикум", Observation{Price: &price, Enrollment: "open", Schedule: "scheduled"}, at)
	if err != nil {
		t.Fatal(err)
	}
	record.Source += "/"
	record.Offers[0].URL += "/"
	record.Offers = append(record.Offers, catalog.Offer{ID: record.ID + "-plus", Name: "Plus", Price: &price, PriceKind: "exact", PriceCheckedAt: at, Enrollment: "open", Schedule: "scheduled", URL: c.URL})
	if err = db.Import(context.Background(), catalog.Dataset{Domains: []string{"practicum.yandex.ru"}, Courses: []catalog.Course{record}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	bindings, _ := json.Marshal(map[string]practicumTariffBinding{c.ExternalID: {OfferID: record.Offers[0].ID}, "python-developer-plus": {OfferID: record.Offers[1].ID}})
	_, err = db.Pool.Exec(context.Background(), "INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id,practicum_tariffs) VALUES('yandex',$1,$2,$3,$4,$5::jsonb)", c.ExternalID, c.URL, record.ID, record.Offers[0].ID, bindings)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(page)))}, nil
	})}
	_, docs, err := aiFetchDocuments(context.Background(), db, client, record)
	if err != nil || len(docs) != 3 {
		t.Fatal(docs, err)
	}
	for _, d := range docs {
		hasPlus := strings.Contains(d.Text, "PLUS_ONLY_MENTOR")
		if d.OfferID == record.Offers[1].ID && !hasPlus || d.OfferID != record.Offers[1].ID && hasPlus {
			t.Fatal("Plus support leaked", d)
		}
	}
}

func TestAILegacyPracticumPublicationStillProtectsTariffMapping(t *testing.T) {
	db := postgresFixture(t)
	ctx := context.Background()
	page, _, _, c := sharedPracticum(t)
	page = []byte(strings.Replace(string(page), "</body>", "<section><h2>Кому подходит</h2><p>"+strings.Repeat(aiBoundTestQuote, 8)+"</p></section></body>", 1))
	price := int64(10000000)
	record, err := normalizedRecord(c, "Python-разработчик", "Python обучение", "Практикум", Observation{Price: &price, Enrollment: "open", Schedule: "scheduled"}, time.Now().UTC().Truncate(time.Microsecond))
	if err != nil {
		t.Fatal(err)
	}
	record.Source += "/"
	record.Offers[0].URL += "/"
	extra := record.Offers[0]
	extra.ID += "-manual"
	extra.Name = "Manual extra"
	record.Offers = append(record.Offers, extra)
	if err = db.Import(ctx, catalog.Dataset{Domains: []string{"practicum.yandex.ru"}, Courses: []catalog.Course{record}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, "INSERT INTO catalog_identities(adapter,external_id,canonical_url,course_id,offer_id) VALUES('yandex',$1,$2,$3,$4)", c.ExternalID, c.URL, record.ID, record.Offers[0].ID); err != nil {
		t.Fatal(err)
	}
	client := clientResponse(200, string(page))
	service := AIService{DB: db, Config: AIConfig{Model: "fixture", BatchSize: 1, DailyLimit: 2}, Model: &basicsAIModel{}, Client: client}
	got, err := service.Run(ctx)
	if err != nil || got.Queued != 1 {
		t.Fatal(got, err)
	}
	pub := Service{DB: db, Config: productionConfig(t), Worker: "publisher"}
	// Mutating the saved binding must reject an otherwise valid cached response.
	db.Pool.Exec(ctx, "UPDATE catalog_identities SET offer_id=$2 WHERE course_id=$1", record.ID, extra.ID)
	result, err := pub.Publish(ctx)
	if err != nil || result.RefreshedCourses != 0 {
		t.Fatal(result, err)
	}
	db.Pool.Exec(ctx, "UPDATE catalog_identities SET offer_id=$2 WHERE course_id=$1", record.ID, record.Offers[0].ID)
	db.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now() WHERE course_id=$1", record.ID)
	got, err = service.Run(ctx)
	if err != nil || got.Cached != 1 {
		t.Fatal(got, err)
	}
	// A malformed changed mapping must not hash like the original NULL mapping.
	if _, err = db.Pool.Exec(ctx, "UPDATE catalog_identities SET practicum_tariffs='[]'::jsonb WHERE course_id=$1", record.ID); err != nil {
		t.Fatal(err)
	}
	result, err = pub.Publish(ctx)
	if err != nil || result.RefreshedCourses != 0 {
		t.Fatal("malformed mapping published", result, err)
	}
	db.Pool.Exec(ctx, "UPDATE catalog_identities SET practicum_tariffs=NULL WHERE course_id=$1", record.ID)
	db.Pool.Exec(ctx, "UPDATE catalog_ai_sources SET next_attempt_at=now() WHERE course_id=$1", record.ID)
	got, err = service.Run(ctx)
	if err != nil || got.Cached != 1 {
		t.Fatal(got, err)
	}
	result, err = pub.Publish(ctx)
	if err != nil || result.RefreshedCourses != 1 {
		t.Fatal(result, err)
	}
	loaded, err := db.Load(ctx)
	if err != nil || loaded[0].Profile == nil || loaded[0].Source != record.Source || *loaded[0].Offers[0].Price != price || !loaded[0].Offers[0].PriceCheckedAt.Equal(record.Offers[0].PriceCheckedAt) {
		t.Fatal(loaded, err)
	}
}
