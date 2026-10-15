package updater

import "time"

// Isolate structurally invalid or ungrounded claims before independent review.
// Partial observations cannot attest disappearance of previously verified facts.
func aiIsolateClaims(c Candidate, docs []AIDocument, x AIExtraction, at time.Time) (AIExtraction, error) {
	if !aiDocumentsValid(c, docs) || x.CourseURL != c.URL || !x.Complete || len(x.Claims) > aiMaxClaims || x.DiscardedClaims != 0 {
		return x, ErrAI
	}
	counts := map[string]int{}
	for _, claim := range x.Claims {
		counts[claim.OfferID+":"+claim.Field]++
	}
	valid := make([]AIClaim, 0, len(x.Claims))
	for _, claim := range x.Claims {
		one := AIExtraction{CourseURL: x.CourseURL, Complete: true, Claims: []AIClaim{claim}}
		_, err := aiCompile(c, docs, one, AIVerification{Complete: true}, at)
		if err != nil || counts[claim.OfferID+":"+claim.Field] != 1 {
			x.DiscardedClaims++
			continue
		}
		valid = append(valid, claim)
	}
	if len(x.Claims) > 0 && len(valid) == 0 {
		return x, ErrAI
	}
	x.Claims = valid
	return x, nil
}
