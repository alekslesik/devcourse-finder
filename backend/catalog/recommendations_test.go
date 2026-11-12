package catalog

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGameDevelopmentIsAnAdvisoryDirectionWithNormalHardFilters(t *testing.T) {
	now := time.Now().UTC()
	c := recommendationFixture(now)
	c.Language = "python"
	c.Title = "Создание игр на Python с PyGame"
	c.Recommendations.Directions.Values = []string{"games"}
	c.Recommendations.Directions.Quote = "Создадите игровые окна, спрайты, обработку ввода и столкновений, пять собственных 2D игр."
	if !c.Recommendations.Valid() {
		t.Fatal("games rejected by recommendation contract")
	}
	f, err := Parse(url.Values{"direction": {"games"}, "language": {"python"}, "include_recommendations": {"true"}})
	if err != nil {
		t.Fatal(err)
	}
	got := Search([]Course{c}, f, now)
	if len(got) != 1 || got[0].ProfileMatch != "recommended" {
		t.Fatal("game course disappeared", got)
	}
	counts := ProfileFacets([]Course{c}, f, now)["direction"]
	if counts.Counts["games"] != 0 || counts.RecommendedCounts["games"] != len(got) {
		t.Fatal("game facet differs from actual matching courses", counts)
	}
	f.IncludeRecommendations = false
	if len(Search([]Course{c}, f, now)) != 0 {
		t.Fatal("advice became a confirmed school fact")
	}
	f.IncludeRecommendations = true
	f.Max = new(int64)
	if len(Search([]Course{c}, f, now)) != 0 {
		t.Fatal("games bypassed price constraint")
	}
	f.Max = nil
	f.Direction = "frontend"
	if len(Search([]Course{c}, f, now)) != 0 {
		t.Fatal("game development leaked into web frontend")
	}
	if RecommendationLabel("direction", "games") != "разработка игр" {
		t.Fatal("missing public label")
	}
}

func recommendationFixture(now time.Time) Course {
	tags := func(values ...string) *RecommendationTags {
		return &RecommendationTags{Values: values, Quote: "В программе практика программирования, проекты и освоение новых навыков.", Evidence: LearningEvidence{Source: "https://school.example/course", Locator: "section:program", Digest: strings.Repeat("a", 64), CheckedAt: now}}
	}
	price := int64(10000)
	known := false
	return Course{ID: "estimate", Slug: "estimate", Title: "Course", Source: "https://school.example/course", Status: "published", CheckedAt: now, Direction: "unknown", Audience: []string{"unknown"}, Goals: []string{"unknown"}, Recommendations: &CourseRecommendations{Audience: tags("none", "basics", "switch"), Goals: tags("try", "job", "switch", "deepen"), Directions: tags("backend", "data")}, Offers: []Offer{{ID: "offer", Price: &price, PriceKind: "exact", PriceCheckedAt: now, Enrollment: "continuous", SupportKnown: &known}}}
}
func TestRecommendationDecisionScenariosKeepFactsAndConstraints(t *testing.T) {
	now := time.Now().UTC()
	c := recommendationFixture(now)
	for _, f := range []Filter{{Experience: "none"}, {Experience: "basics"}, {Experience: "switch"}, {Goal: "job"}, {Direction: "data"}} {
		if len(Search([]Course{c}, f, now)) != 0 {
			t.Fatal("strict search accepted an estimate", f)
		}
		f.IncludeRecommendations = true
		before, _ := json.Marshal(c)
		r := Search([]Course{c}, f, now)
		after, _ := json.Marshal(c)
		if len(r) != 1 || r[0].ProfileMatch != "recommended" || len(r[0].RecommendationFields) != 1 || string(before) != string(after) {
			t.Fatal(f, r)
		}
	}
	for _, f := range []Filter{{Experience: "none", Support: "mentor"}, {Goal: "job", Max: new(int64)}, {Goal: "job", Hours: new(int)}} {
		f.IncludeRecommendations = true
		if len(Search([]Course{c}, f, now)) != 0 {
			t.Fatal("estimate bypassed a hard constraint", f)
		}
	}
	c.Profile = &CourseProfile{Prerequisites: &ProfileRequirement{Level: "basics", Scope: "programming"}}
	if len(Search([]Course{c}, Filter{Experience: "none", IncludeRecommendations: true, IncludeUnknownProfiles: true}, now)) != 0 {
		t.Fatal("estimate overrode explicit prerequisite")
	}
	c.Profile = &CourseProfile{ExcludedAudience: &ProfileTags{Values: []string{"none"}}}
	if MatchRecommendation(c, "experience", "none", now) {
		t.Fatal("estimate overrode excluded audience")
	}
}
func TestRecommendationRankingAbstentionAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	estimate := recommendationFixture(now)
	confirmed := estimate
	confirmed.ID = "confirmed"
	confirmed.Goals = []string{"job"}
	confirmed.Recommendations = nil
	unknown := estimate
	unknown.ID = "unknown"
	unknown.Recommendations = nil
	f := Filter{Goal: "job", IncludeRecommendations: true, IncludeUnknownProfiles: true, Sort: "price_asc"}
	r := Search([]Course{unknown, estimate, confirmed}, f, now)
	if len(r) != 3 || r[0].ProfileMatch != "confirmed" || r[1].ProfileMatch != "recommended" || r[2].ProfileMatch != "unverified" {
		t.Fatal(r)
	}
	facet := ProfileFacets([]Course{unknown, estimate, confirmed}, f, now)["goal"]
	if facet.Counts["job"] != 1 || facet.RecommendedCounts["job"] != 1 {
		t.Fatal(facet)
	}
	if MatchRecommendation(estimate, "goal", "job", now.Add(91*24*time.Hour)) {
		t.Fatal("stale recommendation matched")
	}
	f.Experience = "projects"
	r = Search([]Course{estimate}, f, now)
	if len(r) != 1 || r[0].ProfileMatch != "unverified" || len(r[0].UnknownProfileFields) != 1 || r[0].UnknownProfileFields[0] != "experience" {
		t.Fatal(r)
	}
	bad := *estimate.Recommendations
	bad.Goals = &RecommendationTags{Values: []string{"job"}, Quote: "Invented unverified claim"}
	estimate.Recommendations = &bad
	if MatchRecommendation(estimate, "goal", "job", now) {
		t.Fatal("invalid evidence matched")
	}
}
func TestNewConfirmedFactsPruneDetailRecommendationsWithoutMutatingSnapshot(t *testing.T) {
	now := time.Now().UTC()
	c := recommendationFixture(now)
	c.Profile = &CourseProfile{Prerequisites: &ProfileRequirement{Level: "basics", Scope: "programming"}}
	pruned := RelevantRecommendations(c)
	if pruned == nil || pruned.Audience == nil || len(pruned.Audience.Values) != 2 || pruned.Audience.Values[0] != "basics" || pruned.Audience.Values[1] != "switch" {
		t.Fatal(pruned)
	}
	if len(c.Recommendations.Audience.Values) != 3 {
		t.Fatal("immutable snapshot was mutated")
	}
	c.Audience = []string{"basics"}
	pruned = RelevantRecommendations(c)
	if pruned == nil || pruned.Audience != nil {
		t.Fatal("confirmed/contradictory audience remained an estimate", pruned)
	}
}
