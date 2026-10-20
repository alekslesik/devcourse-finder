package updater

import (
	"encoding/json"
	"sort"
	"strings"

	"devcourse-finder/catalog"
	"golang.org/x/net/html"
)

// The registration-only template identifies a public program/family by slug,
// not the legacy numeric checkout product. It can attest course metadata ONLY.
// Numeric mappings remain unchanged and are rechecked at publication; this path
// never claims that their financial product/price was freshly verified.
func aiNetologyPublicDocuments(raw []byte, c Candidate, current catalog.Course, b aiNetologyBinding, digest string) ([]AIDocument, error) {
	if c.Adapter != "netology" || !aiSourceMatches(current.Source, c) || b.FamilyID <= 0 || b.ProgramID <= 0 {
		return nil, ErrAI
	}
	bound := false
	for _, o := range current.Offers {
		if o.ID == b.OfferID && aiSourceMatches(o.URL, c) {
			bound = true
		}
	}
	if !bound {
		return nil, ErrAI
	}
	doc := learningDocument(raw)
	var payload []byte
	canonicalCount, scriptCount, h1Count := 0, 0, 0
	canonicalOK, russian := false, false
	title, description := "", ""
	visitNodes(doc, func(n *html.Node) {
		switch n.Data {
		case "html":
			russian = nodeAttr(n, "lang") == "ru" || nodeAttr(n, "lang") == "ru-RU"
		case "link":
			if nodeAttr(n, "rel") == "canonical" {
				canonicalCount++
				canonicalOK = matchingURL(nodeAttr(n, "href"), c.URL)
			}
		case "h1":
			h1Count++
			title = nodeText(n)
		case "meta":
			if nodeAttr(n, "name") == "description" {
				description = nodeAttr(n, "content")
			}
		case "script":
			if nodeAttr(n, "id") == "__NEXT_DATA__" {
				scriptCount++
				if nodeAttr(n, "type") == "application/json" && n.FirstChild != nil {
					payload = []byte(n.FirstChild.Data)
				}
			}
		}
	})
	// A removed marketing suffix is allowed only with the independently checked
	// canonical/query/first registration slug below. It never sets audience=none.
	sameTitle := normalizeTitle(title) == normalizeTitle(current.Title) || normalizeTitle(title) == strings.TrimSuffix(normalizeTitle(current.Title), " с нуля")
	if !russian || canonicalCount != 1 || !canonicalOK || h1Count != 1 || !sameTitle || scriptCount != 1 || courseLanguage(title, description) != current.Language {
		return nil, ErrAI
	}
	var page struct {
		Page  string `json:"page"`
		Query struct {
			Name string `json:"programName"`
		} `json:"query"`
		Props struct {
			PageProps struct {
				State map[string]json.RawMessage `json:"initialState"`
			} `json:"pageProps"`
		} `json:"props"`
	}
	if decodeJSON(payload, &page) != nil || page.Query.Name != c.ExternalID || page.Page != "" && page.Page != "/programs/[programName]" {
		return nil, ErrAI
	}
	// A contradictory/partly loaded legacy product must never fall back to this
	// weaker, explicitly course-only contract.
	if _, exists := page.Props.PageProps.State["program"]; exists {
		return nil, ErrAI
	}
	var landing struct {
		Content map[string]json.RawMessage `json:"content"`
	}
	if decodeJSON(page.Props.PageProps.State["landingContent"], &landing) != nil {
		return nil, ErrAI
	}
	registrations := 0
	var texts []string
	if description != "" {
		texts = append(texts, aiPublicText(description))
	}
	keys := make([]string, 0, len(landing.Content))
	for key := range landing.Content {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := landing.Content[key]
		if strings.HasPrefix(key, "programRegistration") {
			registrations++
			var reg struct {
				Visible  *bool `json:"isVisible"`
				Breeding *bool `json:"isBreedingPrograms"`
				Programs []struct {
					Slug string `json:"slug"`
				} `json:"customBreedingPrograms"`
			}
			if decodeJSON(value, &reg) != nil || reg.Visible == nil || !*reg.Visible || reg.Breeding == nil || !*reg.Breeding || len(reg.Programs) < 1 || reg.Programs[0].Slug != c.ExternalID {
				return nil, ErrAI
			}
			seen := map[string]bool{}
			for _, p := range reg.Programs {
				if !slugPattern.MatchString(p.Slug) || seen[p.Slug] {
					return nil, ErrAI
				}
				seen[p.Slug] = true
			}
		}
		if !strings.HasPrefix(key, "programModule_") {
			continue
		}
		var module struct {
			Visible *bool `json:"isVisible"`
			Blocks  []struct {
				Title       string `json:"title"`
				Description string `json:"description"`
				Lessons     []struct {
					Title       string `json:"title"`
					Description string `json:"description"`
				} `json:"lessons"`
			} `json:"blocks"`
		}
		if decodeJSON(value, &module) != nil {
			return nil, ErrAI
		}
		// A retained DOM shell is not evidence that a hidden/unspecified module
		// belongs to the currently published curriculum.
		if module.Visible == nil || !*module.Visible {
			continue
		}
		// Only a module rendered once on this course page belongs to its curriculum.
		roots := learningNodes(doc, func(n *html.Node) bool { return nodeAttr(n, "data-contentkey") == key })
		if len(roots) != 1 {
			return nil, ErrAI
		}
		for _, block := range module.Blocks {
			texts = append(texts, aiPublicText(block.Title), aiPublicText(block.Description))
			for _, lesson := range block.Lessons {
				texts = append(texts, aiPublicText(lesson.Title), aiPublicText(lesson.Description))
			}
		}
	}
	text := aiNormalize(strings.Join(texts, "\n"))
	if registrations != 1 || len(text) < 200 {
		return nil, ErrAI
	}
	docs := []AIDocument{{ID: "course-public", Scope: "course", Partial: true, Source: c.URL, Text: text, Digest: digest, BindingDigest: aiNetologyPublicBindingDigest(current.ID, b)}}
	if !aiDocumentsValid(c, docs) {
		return nil, ErrAI
	}
	return docs, nil
}
func aiNetologyPublicBindingDigest(courseID string, b aiNetologyBinding) string {
	return fingerprint([]byte("netology-public-course-v1\x00" + aiNetologyBindingDigest(courseID, b)))
}
