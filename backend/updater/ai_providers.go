package updater

import "sort"

// Only providers with implemented document/identity contracts belong here.
// Native parser support alone never opts a provider into paid AI enrichment.
var aiProviders = map[string]string{
	"netology": "https://netology.ru/programs/%",
	"stepik":   "https://stepik.org/course/%",
	"yandex":   "https://practicum.yandex.ru/%",
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
