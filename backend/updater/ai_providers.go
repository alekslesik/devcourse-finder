package updater

import (
	"sort"
	"strings"
)

// Only providers with implemented document/identity contracts belong here.
// Native parser support alone never opts a provider into paid AI enrichment.
var aiProviders = map[string]string{
	"codebasics": "https://code-basics.com/ru/languages/%",
	"netology":   "https://netology.ru/programs/%",
	"stepik":     "https://stepik.org/course/%",
	"yandex":     "https://practicum.yandex.ru/%",
}

func aiCandidateURL(raw string) (Candidate, bool) {
	for adapter := range aiProviders {
		if c, ok := candidateURL(adapter, raw, ""); ok && aiSourceMatches(raw, c) {
			if adapter == "htmlacademy" && !strings.HasPrefix(c.ExternalID, "intensive-") {
				continue
			}
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

func aiSourceHosts() []string {
	hosts := make([]string, 0, len(aiProviders))
	for adapter := range aiProviders {
		hosts = append(hosts, providerHosts[adapter])
	}
	sort.Strings(hosts)
	return hosts
}

func aiProviderIDs() []string {
	ids := make([]string, 0, len(aiProviders))
	for id := range aiProviders {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Historical curated Practicum/OTUS records used a single trailing slash. This
// permits only that cosmetic spelling, never queries, fragments or other hosts.
func aiSourceMatches(raw string, c Candidate) bool {
	return raw == c.URL || (c.Adapter == "yandex" || c.Adapter == "otus") && raw == c.URL+"/"
}
