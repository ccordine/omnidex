package cognitionpolicy

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/llm"
)

// CallInputAuthority is the exact code-rendered policy input identity persisted
// with a runtime snapshot before any provider call may be reserved.
type CallInputAuthority struct {
	EnvelopeRendererVersion       string
	EnvelopeTokenEstimator        string
	EnvelopeEstimatedTokens       int
	EnvelopeSHA256                string
	EnvelopeBytes                 int
	PromptHintSHA256              string
	PromptHintBytes               int
	ModelVisibleInputSHA256       string
	ModelVisibleInputBytes        int
	ModelVisibleEstimatedTokens   int
	ModelInputTokenUpperBound     int
	ResponseContractSHA256        string
	ExpectedProviderRequestSHA256 string
}

// MeasureCallInputAuthority derives the exact immutable input and structured
// response contract from runtime state. It performs no provider call.
func MeasureCallInputAuthority(
	snapshot cognition.RuntimeSnapshot,
	projection contextbuilder.Projection,
	brain BrainRef,
) (CallInputAuthority, error) {
	rendered, err := Render(snapshot, projection, brain)
	if err != nil {
		return CallInputAuthority{}, err
	}
	contractSHA, err := responseContractSHA(snapshot.ActionCatalog())
	if err != nil {
		return CallInputAuthority{}, err
	}
	prompt := llm.MinimalGeneratePrompt
	modelInput, err := llm.ExactPreparedModelInput(rendered.JSON, prompt)
	if err != nil {
		return CallInputAuthority{}, err
	}
	prepared, err := exactPreparedAuthorityModel(snapshot, brain, rendered.JSON)
	if err != nil {
		return CallInputAuthority{}, err
	}
	expectedRequestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return CallInputAuthority{}, err
	}
	visibleBytes := len(modelInput)
	tokenUpperBound, err := llm.ModelInputTokenUpperBound(
		modelInput, brain.Sampling.InputSpecialTokenReserve,
	)
	if err != nil {
		return CallInputAuthority{}, err
	}
	input := CallInputAuthority{
		EnvelopeRendererVersion: rendered.Version,
		EnvelopeTokenEstimator:  rendered.TokenEstimator,
		EnvelopeEstimatedTokens: rendered.EstimatedTokens,
		EnvelopeSHA256:          rendered.SHA256, EnvelopeBytes: rendered.Bytes,
		PromptHintSHA256: policySHA256(prompt), PromptHintBytes: len(prompt),
		ModelVisibleInputBytes:        visibleBytes,
		ModelVisibleEstimatedTokens:   estimatePolicyTokens(visibleBytes),
		ModelInputTokenUpperBound:     tokenUpperBound,
		ResponseContractSHA256:        contractSHA,
		ExpectedProviderRequestSHA256: expectedRequestSHA,
	}
	input.ModelVisibleInputSHA256 = modelVisibleInputSHA(CallAttempt{
		Envelope: rendered.JSON, PromptHint: prompt,
	})
	return input, nil
}

// VerifyCallAttempt re-renders the exact model input and response contract
// from durable runtime authority. A self-consistent caller-authored attempt is
// not sufficient evidence that the registered cognition policy produced it.
func VerifyCallAttempt(
	snapshot cognition.RuntimeSnapshot,
	projection contextbuilder.Projection,
	attempt CallAttempt,
) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	rendered, err := Render(snapshot, projection, attempt.Brain)
	if err != nil {
		return err
	}
	expected, err := newCallAttempt(snapshot, AttestedBrain{
		Ref: attempt.Brain, Attestation: attempt.ProviderAttestation,
		Host: attempt.HostHardwareAttestation,
	}, rendered)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(expected, attempt) {
		return fmt.Errorf("%w: call attempt differs from the exact rendered runtime authority", ErrInvalidEvidence)
	}
	return nil
}

func callInputAuthority(attempt CallAttempt) CallInputAuthority {
	return CallInputAuthority{
		EnvelopeRendererVersion: attempt.EnvelopeRendererVersion,
		EnvelopeTokenEstimator:  attempt.EnvelopeTokenEstimator,
		EnvelopeEstimatedTokens: attempt.EnvelopeEstimatedTokens,
		EnvelopeSHA256:          attempt.EnvelopeSHA256, EnvelopeBytes: attempt.EnvelopeBytes,
		PromptHintSHA256: attempt.PromptHintSHA256, PromptHintBytes: attempt.PromptHintBytes,
		ModelVisibleInputSHA256:       attempt.ModelVisibleInputSHA256,
		ModelVisibleInputBytes:        attempt.ModelVisibleInputBytes,
		ModelVisibleEstimatedTokens:   attempt.ModelVisibleEstimatedTokens,
		ModelInputTokenUpperBound:     attempt.ModelInputTokenUpperBound,
		ResponseContractSHA256:        attempt.ResponseContractSHA256,
		ExpectedProviderRequestSHA256: attempt.ExpectedProviderRequestSHA256,
	}
}

func exactPreparedAuthorityModel(
	snapshot cognition.RuntimeSnapshot,
	brain BrainRef,
	envelope string,
) (llm.PreparedModel, error) {
	schemaRaw, err := decisionSchemaJSON(snapshot.ActionCatalog())
	if err != nil {
		return llm.PreparedModel{}, err
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaRaw, &schema); err != nil {
		return llm.PreparedModel{}, fmt.Errorf("decode exact cognition response schema: %w", err)
	}
	zero := 0.0
	return llm.PreparedModel{
		BaseModel: brain.Model, ContextModel: brain.Model,
		Prompt: envelope, PromptHint: llm.MinimalGeneratePrompt,
		MaxOutputTokens: snapshot.Budget().MaxOutputTokens,
		ContextTokens:   brain.NativeContextLimit,
		ResponseFormat:  llm.ResponseFormatJSON, ResponseSchema: schema,
		ThinkingEnabled: false, Temperature: &zero,
	}, nil
}
