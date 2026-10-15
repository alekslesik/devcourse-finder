package updater

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"devcourse-finder/catalog"
)

func aiEventFixture(t *testing.T) (*Service, catalog.Course, AIEvent) {
	t.Helper()
	record, engine, docs := aiImportedCourse(t)
	c, _, x, v := aiFixture()
	return &Service{DB: engine.DB, Config: productionConfig(t), Worker: "publisher"}, record, AIEvent{Version: 1, CourseID: record.ID, IdentityDigest: aiIdentityDigest(record), Candidate: c, ObservedAt: time.Now().UTC().Truncate(time.Microsecond), Model: "gpt-4.1-mini", Policy: aiEffectivePolicy(), Documents: docs, Extraction: x, Verification: v}
}
func TestAIPublisherUpdatesOnlyMetadataAndNativeCannotEraseIt(t *testing.T) {
	s, record, e := aiEventFixture(t)
	ctx := context.Background()
	if err := s.enqueueAI(ctx, e); err != nil {
		t.Fatal(err)
	}
	result, err := s.Publish(ctx)
	if err != nil || result.NewCourses != 0 || result.RefreshedCourses != 1 {
		t.Fatal(result, err)
	}
	loaded, err := s.DB.Load(ctx)
	if err != nil || len(loaded) != 1 {
		t.Fatal(loaded, err)
	}
	c := loaded[0]
	o := c.Offers[0]
	if c.Profile == nil || c.Audience[0] != "none" || c.ProfileOwnership&(profileAudience<<aiOwnershipShift) == 0 || o.LearningConditions == nil || o.Hours != nil || o.LearningConditions.WeeklyWorkload.Max != 12 {
		t.Fatal(c)
	}
	if !c.CheckedAt.Equal(record.CheckedAt) || !o.PriceCheckedAt.Equal(record.Offers[0].PriceCheckedAt) || *o.Price != *record.Offers[0].Price || o.Enrollment != record.Offers[0].Enrollment {
		t.Fatal("AI changed financial freshness")
	}
	applyCourseProfile(&c, nil)
	applyLearningConditions(&o, nil)
	if c.Profile == nil || o.LearningConditions == nil {
		t.Fatal("native cleared AI facts")
	}
	// Successful disappearance clears only AI ownership and does not freshen price.
	e.ObservedAt = e.ObservedAt.Add(time.Second)
	e.Extraction.Claims = []AIClaim{}
	e.Verification.Verified = []int{}
	if err = s.enqueueAI(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = s.DB.Load(ctx)
	if loaded[0].Profile != nil || loaded[0].Offers[0].LearningConditions != nil || loaded[0].Audience[0] != "unknown" {
		t.Fatal("disappeared AI facts retained", loaded)
	}
}
func TestAIWatermarkIndependentOfPriceAndOperatorFactsProtected(t *testing.T) {
	s, record, e := aiEventFixture(t)
	ctx := context.Background()
	// A newer price check is neither permission to change prices nor a reason to
	// discard an independently observed course profile.
	newer := e.ObservedAt.Add(20 * time.Second)
	_, err := s.DB.Pool.Exec(ctx, "UPDATE offers SET price_checked_at=$2 WHERE course_id=$1", record.ID, newer)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.enqueueAI(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ := s.DB.Load(ctx)
	if loaded[0].Profile == nil || !loaded[0].Offers[0].PriceCheckedAt.Equal(newer) {
		t.Fatal(loaded)
	}
	// Importing an explicit operator fact resets all AI ownership.
	edited := loaded[0]
	edited.Profile.Audience.Values = []string{"working"}
	edited.Audience = []string{"working"}
	hours := 5
	edited.Offers[0].Hours = &hours
	edited.Offers[0].LearningConditions = nil
	if err = s.DB.Import(ctx, catalog.Dataset{Domains: []string{"stepik.org"}, Courses: []catalog.Course{edited}}, "operator", false); err != nil {
		t.Fatal(err)
	}
	e.ObservedAt = e.ObservedAt.Add(time.Second)
	if err = s.enqueueAI(ctx, e); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	loaded, _ = s.DB.Load(ctx)
	if loaded[0].Audience[0] != "working" || loaded[0].ProfileOwnership != 0 || *loaded[0].Offers[0].Hours != 5 {
		t.Fatal("operator overwritten", loaded)
	}
}
func TestAIPublicationRollbackPoisonAndStaleIdentity(t *testing.T) {
	s, record, e := aiEventFixture(t)
	ctx := context.Background()
	if err := s.enqueueAI(ctx, e); err != nil {
		t.Fatal(err)
	}
	_, err := s.DB.Pool.Exec(ctx, `CREATE FUNCTION reject_ai_ack() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'reject'; END $$; CREATE TRIGGER reject_ai_ack BEFORE UPDATE ON catalog_ai_observations FOR EACH ROW EXECUTE FUNCTION reject_ai_ack()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err == nil {
		t.Fatal("ack failure accepted")
	}
	loaded, _ := s.DB.Load(ctx)
	if loaded[0].Profile != nil {
		t.Fatal("metadata partially committed")
	}
	_, err = s.DB.Pool.Exec(ctx, "DROP TRIGGER reject_ai_ack ON catalog_ai_observations; DROP FUNCTION reject_ai_ack()")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	// Malformed queue rows must be acknowledged without blocking the next item.
	bad := e
	bad.ObservedAt = e.ObservedAt.Add(time.Second)
	bad.Extraction.CourseURL = "https://evil.example/course"
	raw, _ := json.Marshal(bad)
	if _, err = s.DB.Pool.Exec(ctx, "INSERT INTO catalog_ai_observations(course_id,observed_at,payload) VALUES($1,$2,$3::jsonb)", record.ID, bad.ObservedAt, raw); err != nil {
		t.Fatal(err)
	}
	e.ObservedAt = e.ObservedAt.Add(2 * time.Second)
	if err = s.enqueueAI(ctx, e); err != nil {
		t.Fatal(err)
	}
	_, err = s.DB.Pool.Exec(ctx, "UPDATE courses SET language='go' WHERE id=$1", record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := s.DB.Pool.Query(ctx, "SELECT code FROM catalog_ai_observations ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var codes []string
	for rows.Next() {
		var code string
		rows.Scan(&code)
		codes = append(codes, code)
	}
	if len(codes) != 3 || codes[1] != "invalid_ai_observation" || codes[2] != "protected_ai_identity" {
		t.Fatal(codes)
	}
}

func TestAIPartialPublicationPreservesMissingFactsDatesAndOwnership(t *testing.T) {
	s, _, event := aiEventFixture(t)
	ctx := context.Background()
	if err := s.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	old, err := s.DB.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	beforeAudience := *old[0].Profile.Audience
	beforeWorkload := *old[0].Offers[0].LearningConditions.WeeklyWorkload
	profileOwn, learningOwn := old[0].ProfileOwnership, old[0].Offers[0].LearningOwnership
	event.ObservedAt = event.ObservedAt.Add(time.Second)
	event.Extraction.Claims = event.Extraction.Claims[:1]
	event.Extraction.DiscardedClaims = 1
	event.Verification.Verified = []int{} // The independent reviewer rejected the remaining proposal.
	if err = s.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	after, err := s.DB.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after[0].Profile == nil || after[0].Offers[0].LearningConditions == nil {
		t.Fatal("partial response cleared prior facts")
	}
	a := after[0]
	if !a.Profile.Audience.Evidence.CheckedAt.Equal(beforeAudience.Evidence.CheckedAt) || !a.Offers[0].LearningConditions.WeeklyWorkload.Evidence.CheckedAt.Equal(beforeWorkload.Evidence.CheckedAt) || a.ProfileOwnership != profileOwn || a.Offers[0].LearningOwnership != learningOwn {
		t.Fatal("partial response freshened or changed ownership of preserved facts")
	}
	applyCourseProfile(&a, nil)
	applyLearningConditions(&a.Offers[0], nil)
	if a.Profile == nil || a.Offers[0].LearningConditions == nil {
		t.Fatal("preserved facts lost AI protection")
	}
	// A surviving independently approved fact refreshes while an omitted one
	// retains its original evidence date and ownership.
	event.ObservedAt = event.ObservedAt.Add(time.Second)
	event.Verification.Verified = []int{0}
	if err = s.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	after, err = s.DB.Load(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !after[0].Profile.Audience.Evidence.CheckedAt.Equal(event.ObservedAt) || !after[0].Offers[0].LearningConditions.WeeklyWorkload.Evidence.CheckedAt.Equal(beforeWorkload.Evidence.CheckedAt) {
		t.Fatal("partial update did not isolate freshness", after)
	}
	// An explicitly complete later read still clears disappeared AI facts.
	event.ObservedAt = event.ObservedAt.Add(time.Second)
	event.Extraction.DiscardedClaims = 0
	event.Extraction.Claims = nil
	event.Verification.Verified = nil
	if err = s.enqueueAI(ctx, event); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Publish(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ = s.DB.Load(ctx)
	if after[0].Profile != nil || after[0].Offers[0].LearningConditions != nil {
		t.Fatal("complete disappearance no longer clears AI-owned facts")
	}
}
