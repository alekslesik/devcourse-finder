package updater

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func skyproReactFixture(t *testing.T) ([]byte, Candidate) {
	t.Helper()
	raw, err := os.ReadFile("testdata/current-schools/skypro-react.html")
	if err != nil {
		t.Fatal(err)
	}
	return raw, Candidate{Adapter: "skypro", ExternalID: "react-developer", URL: "https://sky.pro/course/programming/react-developer", FeedID: "skypro-catalog"}
}

func TestSkyproCurrentCurriculumKeepsNestedLessonsAndExcludesOtherScopes(t *testing.T) {
	raw, c := skyproReactFixture(t)
	r, o, err := collectSkypro(raw, c, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	b := aiBoundFixtureBinding(r, o)
	docs, err := aiBuildBoundDocuments(raw, c, r, r, o, b, fingerprint(raw))
	if err != nil || len(docs) != 2 {
		t.Fatal(docs, err)
	}
	if docs[0].Scope != "course" || !docs[0].Partial {
		t.Fatal("selected curriculum attests disappearance", docs[0])
	}
	for _, owned := range []string{"Программа курса", "Модуль 1.", "Основы HTML", "Основы CSS", "семантические"} {
		if !strings.Contains(docs[0].Text, owned) {
			t.Fatal("nested learning text lost", owned)
		}
	}
	for _, foreign := range []string{"ежемесячный платеж", "Стоимость и варианты оплаты", "личной консультации"} {
		if strings.Contains(docs[0].Text, foreign) {
			t.Fatal("another scope entered course document", foreign)
		}
	}
	injected := strings.Replace(string(raw), "</h3>", `</h3><form><p>FOREIGN_FORM: 36 месяцев рассрочки и личный наставник.</p></form><aside class="related-course"><h2>Программа курса</h2><p>FOREIGN_COURSE: учитесь Python за 6 месяцев.</p></aside>`, 1)
	if injected == string(raw) {
		t.Fatal("ineffective fixture mutation")
	}
	next, err := aiBuildBoundDocuments([]byte(injected), c, r, r, o, b, fingerprint([]byte(injected)))
	if err != nil || next[0].Text != docs[0].Text {
		t.Fatal("form/related course changed learning scope", err)
	}
	for _, bad := range []string{
		strings.Replace(string(raw), "Программа курса", "Иная программа", 1),
		strings.Replace(string(raw), "Получите полную программу на личной консультации", "Узнать подробности", 1),
		strings.Replace(string(raw), "</body>", `<div class="t-rec"><h2>Программа курса</h2></div></body>`, 1),
	} {
		if _, err := aiBuildBoundDocuments([]byte(bad), c, r, r, o, b, fingerprint([]byte(bad))); err == nil {
			t.Fatal("ambiguous or unbounded curriculum accepted")
		}
	}
	b.Skypro++
	if _, err := aiBuildBoundDocuments(raw, c, r, r, o, b, fingerprint(raw)); err == nil {
		t.Fatal("changed product binding accepted")
	}
}

func TestSkyproCurrentCurriculumQueueAndPublicationKeepIdentityAndFinancialGuards(t *testing.T) {
	raw, c := skyproReactFixture(t)
	r, o, err := collectSkypro(raw, c, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	aiCheckBoundPostgres(t, raw, c, r, o)
}

func TestSkyproCurrentOfficialCurriculumSource(t *testing.T) {
	if os.Getenv("AI_SKYPRO_CURRENT_SOURCE_CHECK") != "1" {
		t.Skip("opt-in anonymous source preparation; no models or production writes")
	}
	_, c := skyproReactFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client := NewClient()
	defer client.CloseIdleConnections()
	raw, digest, err := fetch(ctx, client, c.URL)
	if err != nil {
		t.Fatal("official source unavailable")
	}
	r, o, err := collectSkypro(raw, c, time.Now().UTC())
	if err != nil {
		t.Fatal("native identity rejected", rejectionCode(err, "native_rejected"))
	}
	docs, err := aiBuildBoundDocuments(raw, c, r, r, o, aiBoundFixtureBinding(r, o), digest)
	if err != nil {
		t.Fatal("current learning scope rejected")
	}
	if !docs[0].Partial || !strings.Contains(docs[0].Text, "React Hooks") {
		t.Fatal("current nested curriculum incomplete")
	}
	t.Logf("course_bytes=%d offer_documents=%d model_requests=0 writes=0; saved production mapping still required", len(docs[0].Text), len(docs)-1)
}
