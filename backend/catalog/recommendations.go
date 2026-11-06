package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Recommendations are interpretations of course content, never school-confirmed
// entry requirements, prices, support or career outcomes.
type RecommendationTags struct {
	Values   []string         `json:"values"`
	Quote    string           `json:"quote"`
	Evidence LearningEvidence `json:"evidence"`
}
type CourseRecommendations struct {
	Audience   *RecommendationTags `json:"audience,omitempty"`
	Goals      *RecommendationTags `json:"goals,omitempty"`
	Directions *RecommendationTags `json:"directions,omitempty"`
}

var recommendationDirections = []string{"basics", "backend", "frontend", "fullstack", "automation", "data", "algorithms", "testing"}

func SearchDirections() []string { return append([]string(nil), recommendationDirections...) }
func (r *CourseRecommendations) Valid() bool {
	if r == nil {
		return true
	}
	for field, tags := range map[string]*RecommendationTags{"experience": r.Audience, "goal": r.Goals, "direction": r.Directions} {
		if tags == nil {
			continue
		}
		allowed := recommendationDirections
		if field == "experience" {
			allowed = experienceValues
		}
		if field == "goal" {
			allowed = goalValues
		}
		if len(tags.Values) == 0 || len(tags.Values) > len(allowed) || utf8.RuneCountInString(tags.Quote) < 12 || utf8.RuneCountInString(tags.Quote) > 1200 || !tags.Evidence.Valid() {
			return false
		}
		seen := map[string]bool{}
		for _, v := range tags.Values {
			if !slices.Contains(allowed, v) || seen[v] {
				return false
			}
			seen[v] = true
		}
	}
	return r.Audience != nil || r.Goals != nil || r.Directions != nil
}
func (r *CourseRecommendations) Tags(field string) *RecommendationTags {
	if r == nil {
		return nil
	}
	switch field {
	case "experience":
		return r.Audience
	case "goal":
		return r.Goals
	case "direction":
		return r.Directions
	}
	return nil
}
func MatchRecommendation(c Course, field, value string, now time.Time) bool {
	if value == "" || MatchProfile(c, field, value) != "unknown" || !c.Recommendations.Valid() {
		return false
	}
	t := c.Recommendations.Tags(field)
	if t == nil || now.Sub(t.Evidence.CheckedAt) > 90*24*time.Hour || t.Evidence.CheckedAt.After(now.Add(time.Minute)) {
		return false
	}
	if field == "experience" && value == "experienced" {
		return slices.ContainsFunc(t.Values, func(v string) bool { return v != "none" })
	}
	return slices.Contains(t.Values, value)
}

// Bind estimates to the exact course/offer identity. Repricing does not change
// suitability, but another title, language, source or tariff must be reanalysed.
func CourseIdentityDigest(c Course) string {
	type offer struct{ ID, Name, URL string }
	offers := []offer{}
	for _, o := range c.Offers {
		offers = append(offers, offer{o.ID, o.Name, o.URL})
	}
	sort.Slice(offers, func(i, j int) bool { return offers[i].ID < offers[j].ID })
	raw, _ := json.Marshal(struct {
		ID, Title, Language, Source string
		Offers                      []offer
	}{c.ID, c.Title, c.Language, c.Source, offers})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func (r *CourseRecommendations) Evidence() []LearningEvidence {
	out := []LearningEvidence{}
	if r == nil {
		return out
	}
	for _, t := range []*RecommendationTags{r.Audience, r.Goals, r.Directions} {
		if t != nil {
			out = append(out, t.Evidence)
		}
	}
	return out
}
func RecommendationLabel(field, value string) string {
	labels := map[string]map[string]string{
		"experience": {"none": "старт с нуля", "basics": "развитие базовых знаний", "projects": "практика своих проектов", "working": "развитие навыков разработчика", "switch": "переход с другого языка", "experienced": "обучение с опытом"},
		"goal":       {"try": "первое знакомство с программированием", "job": "подготовка к первой работе", "switch": "смена языка или направления", "deepen": "углубление знаний"},
		"direction":  {"basics": "основы", "backend": "backend", "frontend": "frontend", "fullstack": "fullstack", "automation": "автоматизация", "data": "данные и машинное обучение", "algorithms": "алгоритмы", "testing": "тестирование"},
	}
	return strings.TrimSpace(labels[field][value])
}

// The same pruning is used for publication and reads after native fact updates,
// so detailed pages cannot show an estimate contradicting confirmed conditions.
func RelevantRecommendations(c Course) *CourseRecommendations {
	if c.Recommendations == nil || !c.Recommendations.Valid() {
		return nil
	}
	out := &CourseRecommendations{}
	for field, tags := range map[string]*RecommendationTags{"experience": c.Recommendations.Audience, "goal": c.Recommendations.Goals, "direction": c.Recommendations.Directions} {
		if tags == nil {
			continue
		}
		copyTags := *tags
		copyTags.Values = []string{}
		for _, v := range tags.Values {
			if MatchProfile(c, field, v) == "unknown" {
				copyTags.Values = append(copyTags.Values, v)
			}
		}
		if len(copyTags.Values) == 0 {
			continue
		}
		switch field {
		case "experience":
			out.Audience = &copyTags
		case "goal":
			out.Goals = &copyTags
		case "direction":
			out.Directions = &copyTags
		}
	}
	if !out.Valid() {
		return nil
	}
	return out
}
