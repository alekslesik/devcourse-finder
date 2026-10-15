package updater

import (
	"context"
	"fmt"
	"net/http"
)

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
