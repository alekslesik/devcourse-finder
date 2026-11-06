package updater

import (
	"regexp"
	"slices"
	"strings"
)

// Explicit counterevidence anywhere in a course document blocks zero-entry
// claims even if both models approve an isolated, contradictory requirement.
// This is a conservative abstention rule, not an inferred positive prerequisite.
var aiNotFromZero = regexp.MustCompile(`(?:не\s+(?:начинаем|начинается|начинать|стартуем|стартует|изучаем)\s+(?:[^.!?;\n]{0,80}\s+)?с\s+(?:полного\s+)?нуля|не\s+с\s+(?:полного\s+)?нуля|с\s+(?:полного\s+)?нуля\s+(?:мы\s+)?не\s+(?:начинаем|стартуем)|not\s+(?:(?:start(?:ing)?|begin(?:ning)?)\s+(?:[a-z ]{0,60}\s+)?)?from\s+(?:scratch|zero)|not\s+(?:a\s+beginner\s+course|for\s+beginners))`)

func aiConflictingZeroEntry(claim AIClaim, docs []AIDocument) bool {
	zero := claim.Field == "prerequisites" && claim.Value == "none" || (claim.Field == "audience" || claim.Field == "recommended_audience") && slices.Contains(claim.Values, "none")
	if !zero {
		return false
	}
	for _, d := range docs {
		if d.Scope == "course" && aiNotFromZero.MatchString(strings.ToLower(aiNormalize(d.Text))) {
			return true
		}
	}
	return false
}
