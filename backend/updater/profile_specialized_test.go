package updater

import (
	"devcourse-finder/catalog"
	"os"
	"testing"
	"time"
)

func TestSpecializedSchoolRecordedProfiles(t *testing.T) {
	for _, a := range []string{"htmlacademy", "purpleschool", "otus", "hexlet", "javarush", "rsschool", "codebasics"} {
		raw, err := os.ReadFile("testdata/profiles/" + a + ".html")
		if err != nil {
			t.Fatal(err)
		}
		r, o := attachHTMLProfile(raw, Candidate{Adapter: a, URL: "https://" + providerHosts[a] + "/course"}, catalog.Course{Language: "python"}, Observation{}, time.Now().UTC())
		if !o.ProfileObserved || !r.Profile.Valid() {
			t.Fatal(a, r.Profile)
		}
		switch a {
		case "otus", "codebasics", "purpleschool":
			if r.Profile == nil || r.Profile.Audience == nil {
				t.Fatal(a, "explicit audience missing")
			}
		case "rsschool":
			if r.Profile == nil || r.Profile.Prerequisites == nil || r.Profile.Prerequisites.Level != "basics" {
				t.Fatal(a, "minimum knowledge missing")
			}
			if r.Profile.Audience != nil {
				t.Fatal("open admission is not beginner suitability")
			}
		case "hexlet", "javarush":
			if r.Profile != nil {
				t.Fatal(a, "marketing is not a profile", r.Profile)
			}
		}
	}
}
func TestSpecializedProfileIgnoresAdjacentAndConditionalClaims(t *testing.T) {
	for _, a := range []string{"htmlacademy", "purpleschool", "otus", "hexlet", "javarush", "rsschool", "codebasics"} {
		h := profileContracts[a]
		c := Candidate{Adapter: a, URL: "https://" + providerHosts[a] + "/course"}
		raw := []byte("<nav><h2>" + h.Audience[0] + "</h2><p>Для начинающих без опыта.</p></nav><section class='related-course'><h2>" + h.Audience[0] + "</h2><p>Для опытных разработчиков.</p></section>")
		p := profileFromSections(raw, c, "python", time.Now().UTC(), h)
		if len(p.Evidence()) != 0 {
			t.Fatal(a, p)
		}
	}
	proof := catalog.LearningEvidence{}
	if profileRequirement("Будет плюсом, если вы знаете основы. Это желательно.", "python", proof) != nil {
		t.Fatal("optional became mandatory")
	}
	if profileTags("Не для начинающих без опыта", proof, "audience") != nil {
		t.Fatal("negated audience")
	}
}
