package updater

import (
	"devcourse-finder/catalog"
	"encoding/json"
	"golang.org/x/net/html"
	"strings"
	"time"
)

func init() {
	aiProviders["otus"] = "https://otus.ru/lessons/%"
	aiBoundReaders["otus"] = aiOtusDocuments
}

// Reapply the native single-Course/Offer acceptance contract independently to
// each JSON-LD candidate. Only the one whose normalized identity matched is read.
func aiStructuredNode(raw []byte, c Candidate) (map[string]any, error) {
	var nodes []map[string]any
	for _, m := range jsonLD.FindAllSubmatch(raw, -1) {
		var v any
		if decodeJSON(m[1], &v) != nil {
			return nil, ErrAI
		}
		graphNodes(v, &nodes)
	}
	var selected map[string]any
	for _, n := range nodes {
		b, _ := json.Marshal(n)
		_, _, e := collectStructured([]byte(`<script type="application/ld+json">`+string(b)+`</script>`), c, time.Now().UTC())
		if e == nil {
			if selected != nil {
				return nil, ErrAI
			}
			selected = n
		}
	}
	if selected == nil {
		return nil, ErrAI
	}
	return selected, nil
}
func aiSchemaCalendar(node map[string]any) string {
	instance, ok := node["hasCourseInstance"].(map[string]any)
	if !ok || instance["@type"] != "CourseInstance" {
		return ""
	}
	// Preserve original source wording; do not synthesize a duration from dates,
	// workload totals or subscription periods. Unsupported claims are discarded.
	return schemaString(instance, "duration")
}
func aiOtusDocuments(raw []byte, c Candidate, r catalog.Course) (string, map[string]string, error) {
	node, e := aiStructuredNode(raw, c)
	if e != nil {
		return "", nil, e
	}
	scope := learningOne(learningDocument(raw), func(n *html.Node) bool { return nodeAttr(n, "id") == "vdescription" })
	text := strings.Join([]string{aiPublicText(schemaString(node, "description")), aiCourseSections(raw, c)}, " ")
	return text, map[string]string{r.Offers[0].ID: aiSchemaCalendar(node) + " " + aiOfferText(scope)}, nil
}
