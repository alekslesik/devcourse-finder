package updater

import (
	"devcourse-finder/catalog"
	"golang.org/x/net/html"
	"strconv"
	"time"
)

func skillboxLearning(data []byte, doc *html.Node, r catalog.Course, o Observation, c Candidate, now time.Time) (catalog.Course, Observation) {
	program := learningOne(doc, func(n *html.Node) bool { return n.Data == "section" && nodeAttr(n, "id") == "program" })
	proof := learningProof(data, c.URL, "#program .program-v6__stat", now)
	duration := learningUniqueRange(learningNodes(program, func(n *html.Node) bool { return classToken(n, "program-v6__stat") }), false, proof)
	cards := learningNodes(doc, func(n *html.Node) bool { return n.Data == "li" && nodeAttr(n, "data-tariff-data") != "" })
	by := map[string]*catalog.LearningConditions{}
	for _, card := range cards {
		var v struct {
			ID int64 `json:"nomenclature_id"`
		}
		if decodeJSON([]byte(nodeAttr(card, "data-tariff-data")), &v) != nil {
			continue
		}
		id := r.ID + "-t" + strconv.FormatInt(v.ID, 10)
		p := learningProof(data, c.URL, learningLocator("li[data-tariff-data].nomenclature_id", v.ID), now)
		features := learningNodes(card, func(n *html.Node) bool { return classToken(n, "tariffs-v5__features-inner") })
		text := ""
		if len(features) == 1 {
			text = nodeText(features[0])
		}
		facts := learningSupportText(text, p)
		facts.Duration = duration
		by[id] = facts
	}
	return learningAttach(r, o, by)
}
