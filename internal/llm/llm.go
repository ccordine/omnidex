package llm

import (
	"context"
	"fmt"
	"math"
	"strings"
)

const MinimalGeneratePrompt = "Return only the requested output."

const ResponseFormatJSON = "json"

type PreparedModel struct {
	Protocol                     ExactPreparedProtocol
	BaseModel                    string
	ContextModel                 string
	ModelfilePath                string
	PromptHint                   string
	Prompt                       string
	MaxOutputTokens              int
	ContextTokens                int
	ResponseFormat               string
	ResponseSchema               map[string]any
	ThinkingEnabled              bool
	Temperature                  *float64
	RawTextStopSequence          string
	ProviderIdentityExpectation  *ProviderIdentityExpectation
	ProviderObservationChallenge string
}

func ValidateResponseContract(prepared PreparedModel) error {
	if prepared.Temperature != nil &&
		(math.IsNaN(*prepared.Temperature) || math.IsInf(*prepared.Temperature, 0) ||
			*prepared.Temperature < 0 || *prepared.Temperature > 2) {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	if prepared.ThinkingEnabled && (prepared.ResponseFormat != "" || len(prepared.ResponseSchema) > 0) {
		return fmt.Errorf("native thinking forbids a structured response contract")
	}
	if prepared.ResponseFormat != "" && prepared.ResponseFormat != ResponseFormatJSON {
		return fmt.Errorf("unsupported response format %q", prepared.ResponseFormat)
	}
	if len(prepared.ResponseSchema) == 0 {
		return nil
	}
	if prepared.ResponseFormat != ResponseFormatJSON {
		return fmt.Errorf("response schema requires response format %q", ResponseFormatJSON)
	}
	schemaType, ok := prepared.ResponseSchema["type"].(string)
	if !ok || strings.TrimSpace(schemaType) == "" {
		return fmt.Errorf("response schema requires a non-empty type")
	}
	return nil
}

type EmbeddingClient interface {
	Embedding(ctx context.Context, content string) ([]float64, error)
}

type ExactStationClient interface {
	ExactPreparedContractClient
	ProviderIdentityEvidenceDiscoverer
}
