package cognitionpolicy

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

const (
	SamplingSchemaV1         = "omnidex.cognition-policy-sampling.v1"
	DecisionSchemaProtocolV1 = "omnidex.cognition-decision-schema.v1"
	fixedTemperature         = "0"
	fixedResponseFormat      = "json"
)

type SamplingIdentity struct {
	Schema                string `json:"schema"`
	Temperature           string `json:"temperature"`
	ThinkingEnabled       bool   `json:"thinking_enabled"`
	ResponseFormat        string `json:"response_format"`
	ResponseSchemaVersion string `json:"response_schema_version"`
	NativeContextLimit    int    `json:"native_context_limit"`
	ContextCeilingBytes   int    `json:"context_ceiling_bytes"`
	MaxOutputTokens       int    `json:"max_output_tokens"`
}

func NewSamplingIdentity(
	nativeContextLimit int,
	contextCeilingBytes int,
	maxOutputTokens int,
) (SamplingIdentity, error) {
	identity := SamplingIdentity{
		Schema: SamplingSchemaV1, Temperature: fixedTemperature,
		ThinkingEnabled: false, ResponseFormat: fixedResponseFormat,
		ResponseSchemaVersion: DecisionSchemaProtocolV1,
		NativeContextLimit:    nativeContextLimit, ContextCeilingBytes: contextCeilingBytes,
		MaxOutputTokens: maxOutputTokens,
	}
	if err := identity.Validate(); err != nil {
		return SamplingIdentity{}, err
	}
	return identity, nil
}

func (identity SamplingIdentity) Validate() error {
	if identity.Schema != SamplingSchemaV1 || identity.Temperature != fixedTemperature ||
		identity.ThinkingEnabled || identity.ResponseFormat != fixedResponseFormat ||
		identity.ResponseSchemaVersion != DecisionSchemaProtocolV1 {
		return fmt.Errorf("%w: sampling protocol is not the fixed cognition contract", ErrInvalidBrain)
	}
	if identity.NativeContextLimit <= 0 || identity.NativeContextLimit > MaxNativeContextLimit ||
		identity.ContextCeilingBytes <= 0 || identity.ContextCeilingBytes > MaxContextCeilingBytes ||
		identity.MaxOutputTokens <= 0 || identity.MaxOutputTokens > identity.NativeContextLimit {
		return fmt.Errorf("%w: sampling limits are outside registered bounds", ErrInvalidBrain)
	}
	available, _, err := llm.InferenceInputByteBudget(
		identity.NativeContextLimit, identity.MaxOutputTokens,
	)
	if err != nil || identity.ContextCeilingBytes+len(llm.MinimalGeneratePrompt) > available {
		return fmt.Errorf(
			"%w: context byte ceiling cannot fit the prepared inference boundary", ErrInvalidBrain,
		)
	}
	return nil
}

func (identity SamplingIdentity) SHA256() (string, error) {
	if err := identity.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("%w: encode sampling identity: %v", ErrInvalidBrain, err)
	}
	return policySHA256(string(raw)), nil
}
