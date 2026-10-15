package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

func aiDeepSeekOutputInstructions(input any) string {
	if m, ok := input.(map[string]any); ok {
		if _, verifying := m["proposed_claims"]; verifying {
			return `JSON output contract: {"complete":true,"verified_claim_ids":[zero-based integer indices]}. Shape example with two hypothetical supported claims: {"complete":true,"verified_claim_ids":[0,2]}; if none are supported, return an empty array. These example indices are not decisions about the input. Evaluate each claim independently. Values are internal classification codes, not words that must appear literally in Russian text: prerequisites=none with scope=programming means no programming experience is required; audience=["none"] means explicitly suitable for people who have never programmed. For example, an explicit statement that no special skills or experience are needed can establish prerequisites=none; a generic beginner title alone cannot. Do not require the English code "none" in the source. Include every supported index and no unsupported index. Never approve a claim just to make a nonempty result. No other keys.`
		}
	}
	// List each field's value contract once. Documents already contain all exact
	// document/segment/tariff IDs, so repeating their enum for every field is noise.
	contracts := map[string]any{}
	for _, field := range []string{"prerequisites", "audience", "excluded_audience", "goals", "direction", "target_level", "duration", "weekly_workload", "review", "mentor", "community"} {
		properties := aiClaimSchema(field)["properties"].(map[string]any)
		values := map[string]any{}
		for _, key := range []string{"value", "values", "scope", "min", "max", "unit", "available"} {
			values[key] = properties[key]
		}
		contracts[field] = values
	}
	encoded, _ := json.Marshal(contracts)
	return `JSON output contract: {"course_url":"copy exact input course_url","complete":true,"claims":[claim objects]}. Every claim has only field,offer_id,document_id,quote_id,value,values,scope,min,max,unit,available. Shape example: {"course_url":"exact supplied URL","complete":true,"claims":[{"field":"audience","offer_id":"","document_id":"course","quote_id":"s4","value":"","values":["none"],"scope":"","min":null,"max":null,"unit":"","available":null}]}. This is a shape example, not evidence: never copy its values or IDs without finding them in the input. First find the exact supporting statement, then copy the ID of its actual segment. Never default quote_id to s0. Numeric values must occur literally in the chosen matching OFFER segment; values found elsewhere in the COURSE cannot be copied into a tariff claim. A price/installment block mentioning 36 payments cannot prove 8 months of study or 15 hours a week. Omit unsupported claims. Do not output quote text or discarded_claims. Irrelevant strings are "", values=[], min/max/available=null. Field contracts: ` + string(encoded)
}

// DeepSeekModel uses a fixed provider endpoint and the same grounded fact contract.
type DeepSeekModel struct {
	Key         string `json:"-"`
	Model       string
	VerifyModel string
	Client      *http.Client
}

func (m *DeepSeekModel) client() *OpenAIModel {
	return &OpenAIModel{Key: m.Key, Model: m.Model, VerifyModel: m.VerifyModel, Client: m.Client, provider: "deepseek"}
}

func (m *DeepSeekModel) Extract(ctx context.Context, c Candidate, docs []AIDocument) (AIExtraction, AIUsage, error) {
	return m.client().Extract(ctx, c, docs)
}

func (m *DeepSeekModel) Verify(ctx context.Context, c Candidate, docs []AIDocument, x AIExtraction) (AIVerification, AIUsage, error) {
	return m.client().Verify(ctx, c, docs, x)
}

func (m *DeepSeekModel) String() string { return fmt.Sprintf("DeepSeekModel(%s)", m.Model) }
