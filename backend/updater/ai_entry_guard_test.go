package updater

import (
	"testing"
	"time"
)

func TestAIRejectsZeroEntryDespiteModelApproval(t *testing.T) {
	for _, counter := range []string{"Здесь мы не начинаем с нуля изучать Python.", "Курс не с нуля: это продолжение первой части.", "Обучение с нуля мы не начинаем.", "Мы не начинаем обучение с полного нуля.", "We do not start from scratch.", "This is not a beginner course."} {
		for _, claim := range []AIClaim{{Field: "prerequisites", Value: "none", Scope: "programming"}, {Field: "prerequisites", Value: "none", Scope: "course_language"}, {Field: "audience", Values: []string{"none", "basics"}}, {Field: "recommended_audience", Values: []string{"none"}}} {
			t.Run(counter+claim.Field+claim.Scope, func(t *testing.T) {
				c, docs, _, _ := aiFixture()
				quote := "Требования: Умение программировать с нуля не требуется — всему необходимому мы научимся в процессе."
				for i := range docs {
					docs[i].Text = counter + " " + quote
					docs[i].Digest = fingerprint([]byte(docs[i].Text))
				}
				claim.DocumentID = "course"
				claim.Quote = quote
				x := AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{claim}}
				if _, err := aiCompile(c, docs, x, AIVerification{Complete: true, Verified: []int{0}}, time.Now()); err == nil {
					t.Fatal("contradictory zero-entry claim escaped model-independent guard")
				}
			})
		}
	}
}
func TestAIEntryGuardPreservesOtherFieldsAndPositiveBeginners(t *testing.T) {
	c, docs, _, _ := aiFixture()
	quote := "Программа содержит практические задания на алгоритмы и структуры данных."
	text := "Здесь мы не начинаем с нуля изучать Python. Умение программировать не требуется. " + quote
	for i := range docs {
		docs[i].Text = text
		docs[i].Digest = fingerprint([]byte(text))
	}
	x := AIExtraction{CourseURL: c.URL, Complete: true, Claims: []AIClaim{{Field: "prerequisites", DocumentID: "course", Quote: "Умение программировать не требуется.", Value: "none", Scope: "programming"}, {Field: "recommended_directions", DocumentID: "course", Quote: quote, Values: []string{"algorithms"}}}}
	filtered, err := aiIsolateClaims(c, docs, x, time.Now())
	if err != nil || filtered.DiscardedClaims != 1 || len(filtered.Claims) != 1 {
		t.Fatal(filtered, err)
	}
	facts, err := aiCompile(c, docs, filtered, AIVerification{Complete: true, Verified: []int{0}}, time.Now())
	if err != nil || facts.Profile != nil || facts.Recommendations == nil {
		t.Fatal(facts, err)
	}
	// Ordinary positive beginner wording must still pass the same compiler.
	for _, text := range []string{"Мы начинаем с нуля. Опыт программирования не требуется.", "Не бойтесь начать с нуля. Опыт программирования не требуется."} {
		for i := range docs {
			docs[i].Text = text
			docs[i].Digest = fingerprint([]byte(text))
		}
		x.Claims = []AIClaim{{Field: "prerequisites", DocumentID: "course", Quote: "Опыт программирования не требуется.", Value: "none", Scope: "programming"}}
		if _, err := aiCompile(c, docs, x, AIVerification{Complete: true, Verified: []int{0}}, time.Now()); err != nil {
			t.Fatal(text, err)
		}
	}
	// Counterevidence on an offer-only document cannot change course suitability.
	docs[1].Text = "Курс не с нуля."
	docs[1].Digest = fingerprint([]byte(docs[1].Text))
	if _, err := aiCompile(c, docs, x, AIVerification{Complete: true, Verified: []int{0}}, time.Now()); err != nil {
		t.Fatal("offer context leaked into course guard", err)
	}
}
