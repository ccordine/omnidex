package llm

import (
	"context"
	"fmt"
	"math"
)

const MinimalGeneratePrompt = "Return only the requested output."

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
	Protocol            ExactPreparedProtocol
	BaseModel           string
	ContextModel        string
	PromptHint          string
	Prompt              string
	MaxOutputTokens     int
	OutputLimitMode     ExactPreparedOutputLimitMode
	ContextTokens       int
	Temperature         *ExactPreparedTemperature
	RawTextStopSequence string
}

func ValidateResponseContract(prepared PreparedModel) error {
	if prepared.Temperature != nil &&
		(math.IsNaN(float64(*prepared.Temperature)) || math.IsInf(float64(*prepared.Temperature), 0) ||
			*prepared.Temperature < 0 || *prepared.Temperature > 2) {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	return nil
}

type EmbeddingClient interface {
	Embedding(ctx context.Context, content string) ([]float64, error)
}

type ExactStationClient interface {
	ExactPreparedContractClient
}
