package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
)

type ExactPreparedOutputLimitMode string

// ExactPreparedTemperature is an exact provider sampling value. Callers may
// use only the profile's registered baseline and exploration sequence.
type ExactPreparedTemperature float64

const (
	ExactPreparedOutputLimitExplicit ExactPreparedOutputLimitMode = "explicit"
)

func (mode ExactPreparedOutputLimitMode) Validate() error {
	switch mode {
	case ExactPreparedOutputLimitExplicit:
		return nil
	default:
		return fmt.Errorf("exact prepared output-limit mode is not registered")
	}
}

type PreparedModel struct {
	Protocol        ExactPreparedProtocol
	BaseModel       string
	ContextModel    string
	Prompt          string
	MaxOutputTokens int
	OutputLimitMode ExactPreparedOutputLimitMode
	ContextTokens   int
	// RetainedContext contains only token IDs returned by the exact parent
	// source-generation response. Independent station calls leave it nil.
	RetainedContext         []int
	RetainedProviderContext json.RawMessage
	RetainProviderContext   bool
	Temperature             *ExactPreparedTemperature
	RawTextStopSequence     string
}

// NativeRequestPreparer declares provider-specific controls before the queue
// freezes the exact request. It cannot dispatch inference or execute work.
type NativeRequestPreparer interface {
	PrepareExactRequest(PreparedModel) (PreparedModel, error)
}

func PrepareNativeRequest(client ExactStationClient, prepared PreparedModel) (PreparedModel, error) {
	if native, ok := client.(NativeRequestPreparer); ok {
		return native.PrepareExactRequest(prepared)
	}
	return prepared, nil
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
