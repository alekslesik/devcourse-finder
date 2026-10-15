package updater

import (
	"devcourse-finder/catalog"
	"testing"
	"time"
)

func TestAIIsolatesNumbersBorrowedFromAnotherScope(t *testing.T) {
	c, docs, x, _ := aiFixture()
	price := "Go-разработчик с нуля. 6 287 рублей в месяц на 36 месяцев или 154 000 рублей одним платежом."
	docs = append(docs, AIDocument{ID: "price", Source: c.URL, Scope: "offer", OfferID: "other-offer", Text: price, Digest: fingerprint([]byte(price))})
	x.Claims = append(x.Claims,
		AIClaim{Field: "duration", DocumentID: "price", OfferID: "other-offer", Quote: price, Min: valueFloat(8), Max: valueFloat(8), Unit: "month"},
		AIClaim{Field: "weekly_workload", DocumentID: "price", OfferID: "other-offer", Quote: price, Min: valueFloat(15), Max: valueFloat(15), Unit: "hours_per_week"})
	filtered, err := aiIsolateClaims(c, docs, x, time.Now())
	if err != nil || filtered.DiscardedClaims != 2 || len(filtered.Claims) != 2 {
		t.Fatal(filtered, err)
	}
	facts, err := aiCompile(c, docs, filtered, AIVerification{Complete: true, Verified: []int{0}}, time.Now())
	if err != nil || facts.Profile == nil || facts.Learning["other-offer"] != nil || facts.Learning[x.Claims[1].OfferID] != nil {
		t.Fatal("unverified or ungrounded claims escaped independent verification", facts, err)
	}
}

func TestAIIsolationDoesNotRepairWrongIdentityOrAcceptOnlyInvalidClaims(t *testing.T) {
	c, docs, x, _ := aiFixture()
	x.CourseURL += "/another"
	if _, err := aiIsolateClaims(c, docs, x, time.Now()); err == nil {
		t.Fatal("identity repaired")
	}
	x.CourseURL = c.URL
	x.Claims = []AIClaim{x.Claims[1]}
	x.Claims[0].Max = valueFloat(999)
	if _, err := aiIsolateClaims(c, docs, x, time.Now()); err == nil {
		t.Fatal("empty partial observation accepted")
	}
	x.Claims = nil
	if out, err := aiIsolateClaims(c, docs, x, time.Now()); err != nil || out.DiscardedClaims != 0 {
		t.Fatal(out, err)
	}
}

func TestAIIsolationDropsBothConflictingDuplicateFields(t *testing.T) {
	c, docs, x, _ := aiFixture()
	x.Claims = append(x.Claims, x.Claims[0])
	out, err := aiIsolateClaims(c, docs, x, time.Now())
	if err != nil || out.DiscardedClaims != 2 || len(out.Claims) != 1 || out.Claims[0].Field != "weekly_workload" {
		t.Fatal(out, err)
	}
}

func TestAIPartialPreservesNativeAndOperatorOwnership(t *testing.T) {
	candidate, docs, x, v := aiFixture()
	facts, err := aiCompile(candidate, docs, x, v, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, native := range []bool{false, true} {
		c := catalog.Course{Profile: facts.Profile, Audience: []string{"none"}}
		o := catalog.Offer{LearningConditions: facts.Learning[docs[1].OfferID]}
		if native {
			c.ProfileOwnership = profileAudience
			o.LearningOwnership = learningWorkload
		}
		profileOwn, learningOwn := c.ProfileOwnership, o.LearningOwnership
		applyAIProfile(&c, nil, true)
		applyAILearning(&o, nil, true)
		if c.ProfileOwnership != profileOwn || o.LearningOwnership != learningOwn || c.Profile.Audience != facts.Profile.Audience || o.LearningConditions.WeeklyWorkload != facts.Learning[docs[1].OfferID].WeeklyWorkload {
			t.Fatal("preservation changed native/operator facts or ownership")
		}
		// The partial AI event must not acquire ownership of preserved native fields.
		if native {
			applyCourseProfile(&c, nil)
			applyLearningConditions(&o, nil)
			if c.Profile != nil || o.LearningConditions != nil {
				t.Fatal("AI protected a merely preserved native field")
			}
		}
	}
}
