package updater

import "sort"

// Only providers with implemented document/identity contracts belong here.
// Native parser support alone never opts a provider into paid AI enrichment.
var aiProviders = map[string]string{
	"stepik": "https://stepik.org/course/%",
	"yandex": "https://practicum.yandex.ru/%",
}

func aiCandidateURL(raw string) (Candidate, bool) {
	for adapter := range aiProviders {
		if c, ok := candidateURL(adapter, raw, ""); ok && c.URL == raw {
			return c, true
		}
	}
	return Candidate{}, false
}

func aiSourcePatterns() []string {
	patterns := make([]string, 0, len(aiProviders))
	for _, pattern := range aiProviders {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	return patterns
}
