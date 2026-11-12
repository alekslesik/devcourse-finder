package updater

import (
	"os"
	"strings"
	"testing"
)

func TestAISkillfactoryCurrentLearningWrappersStayWithinCourseScope(t *testing.T) {
	raw, pricing, c, r, b := sfPublicFixture(t)
	excerpt, err := os.ReadFile("testdata/current-schools/skillfactory-learning.html")
	if err != nil {
		t.Fatal(err)
	}
	extra := `<section class="tariffs"><h2>Что изучите за 10 месяцев</h2><p>FOREIGN_TARIFF: персональный наставник и рассрочка 36 месяцев.</p></section><section class="related-courses"><h2>Чем занимается Python-разработчик</h2><p>FOREIGN_PROGRAM: другая программа с иными условиями.</p></section>`
	raw = []byte(strings.Replace(string(raw), "</html>", string(excerpt)+extra+"</html>", 1))
	docs, err := aiSkillfactoryPublic(raw, pricing, c, r, b, fingerprint(raw))
	if err != nil || len(docs) != 1 || docs[0].Scope != "course" || !docs[0].Partial {
		t.Fatal(docs, err)
	}
	for _, owned := range []string{"веб-разработке", "Flask и Django", "выпускной работы"} {
		if !strings.Contains(docs[0].Text, owned) {
			t.Fatal("current curriculum lost", owned, docs[0].Text)
		}
	}
	for _, foreign := range []string{"FOREIGN_TARIFF", "FOREIGN_PROGRAM", "рассрочка 36"} {
		if strings.Contains(docs[0].Text, foreign) {
			t.Fatal("other scope leaked", foreign, docs[0].Text)
		}
	}
	if aiSkillfactoryCurrentSections(excerpt, Candidate{Adapter: "skillfactory", ExternalID: "other"}) != "" {
		t.Fatal("unverified course template enabled")
	}
	// Semantic content cannot substitute for the saved independent product identity.
	b.Verified.CourseID++
	if _, err := aiSkillfactoryPublic(raw, pricing, c, r, b, fingerprint(raw)); err == nil {
		t.Fatal("curriculum bypassed independent identity")
	}
}
