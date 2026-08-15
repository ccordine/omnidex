package llm

import (
	"context"
	"fmt"
	"math"
	"strings"
)

const MinimalGeneratePrompt = "Return only the requested output."

const ResponseFormatJSON = "json"

type ExactPreparedOutputLimitMode string

// ExactPreparedTemperature is an exact provider sampling value. Callers may
// use only the profile's registered baseline and exploration sequence.
type ExactPreparedTemperature float64

const (
	ExactPreparedOutputLimitExplicit ExactPreparedOutputLimitMode = "explicit"
	ExactPreparedOutputLimitNatural  ExactPreparedOutputLimitMode = "natural"
)

func (mode ExactPreparedOutputLimitMode) Validate() error {
	switch mode {
	case ExactPreparedOutputLimitExplicit, ExactPreparedOutputLimitNatural:
		return nil
	default:
		return fmt.Errorf("exact prepared output-limit mode is not registered")
	}
}

type PreparedModel struct {
	Protocol                     ExactPreparedProtocol
	BaseModel                    string
	ContextModel                 string
	ModelfilePath                string
	PromptHint                   string
	Prompt                       string
	MaxOutputTokens              int
	OutputLimitMode              ExactPreparedOutputLimitMode
	ContextTokens                int
	ResponseFormat               string
	ResponseSchema               map[string]any
	ThinkingEnabled              bool
	Temperature                  *ExactPreparedTemperature
	RawTextStopSequence          string
	ProviderIdentityExpectation  *ProviderIdentityExpectation
	ProviderObservationChallenge string
}

func ValidateResponseContract(prepared PreparedModel) error {
	if prepared.Temperature != nil &&
		(math.IsNaN(float64(*prepared.Temperature)) || math.IsInf(float64(*prepared.Temperature), 0) ||
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
