package cognitionpolicy

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/contextbuilder"
	"github.com/gryph/omnidex/internal/llm"
)

const decisionInstruction = "Select exactly one registered action for the current obligation. Return only one JSON object matching decision_schema. Do not assign authoritative identifiers or declare completion."

const policyTokenEstimator = contextbuilder.TokenEstimatorV1

type RenderedEnvelope struct {
	Version         string
	JSON            string
	SHA256          string
	Bytes           int
	EstimatedTokens int
	TokenEstimator  string
}

type policyEnvelope struct {
	Schema           string                  `json:"schema"`
	RendererVersion  string                  `json:"renderer_version"`
	Instruction      string                  `json:"instruction"`
	ActionCatalog    cognition.ActionCatalog `json:"action_catalog"`
	EvidenceRefs     []cognition.EvidenceRef `json:"evidence_refs"`
	ProjectedContext json.RawMessage         `json:"projected_context"`
	DecisionSchema   json.RawMessage         `json:"decision_schema"`
}

func Render(
	snapshot cognition.RuntimeSnapshot,
	projection contextbuilder.Projection,
	brain BrainRef,
) (RenderedEnvelope, error) {
	if err := brain.Validate(); err != nil {
		return RenderedEnvelope{}, err
	}
	if err := ValidateRuntimeBudget(brain, snapshot.Budget()); err != nil {
		return RenderedEnvelope{}, err
	}
	envelope, err := MeasureEnvelope(snapshot, projection)
	if err != nil {
		return RenderedEnvelope{}, err
	}
	if err := validateProjectionLimits(projection, brain); err != nil {
		return RenderedEnvelope{}, err
	}
	if snapshot.Budget().MaxOutputTokens > brain.Sampling.MaxOutputTokens {
		return RenderedEnvelope{}, fmt.Errorf(
			"%w: runtime output limit exceeds the frozen cognition station ceiling", ErrEnvelopeLimit,
		)
	}
	if envelope.Bytes > brain.ContextCeilingBytes ||
		envelope.Bytes > snapshot.Budget().MaxInputBytes ||
		envelope.EstimatedTokens > snapshot.Budget().MaxInputTokens ||
		snapshot.Budget().MaxInputTokens+snapshot.Budget().MaxOutputTokens > brain.NativeContextLimit {
		return RenderedEnvelope{}, fmt.Errorf(
			"%w: envelope is %d bytes/%d estimated tokens; hard maximum is %d bytes, call limits are %d bytes/%d input tokens plus %d output tokens, and brain limits are %d bytes/%d native units",
			ErrEnvelopeLimit, envelope.Bytes, envelope.EstimatedTokens, MaxEnvelopeBytes,
			snapshot.Budget().MaxInputBytes, snapshot.Budget().MaxInputTokens,
			snapshot.Budget().MaxOutputTokens,
			brain.ContextCeilingBytes, brain.NativeContextLimit,
		)
	}
	if err := llm.ValidateInferenceBudget(
		brain.NativeContextLimit, snapshot.Budget().MaxOutputTokens,
		envelope.JSON, llm.MinimalGeneratePrompt,
	); err != nil {
		return RenderedEnvelope{}, fmt.Errorf("%w: %v", ErrEnvelopeLimit, err)
	}
	return envelope, nil
}

// MeasureEnvelope renders the exact immutable bytes used by Policy without
// applying a BrainRef or the episode input ceiling. Coordinators use this seam
// to fit projected material under the total envelope budget before persistence.
func MeasureEnvelope(
	snapshot cognition.RuntimeSnapshot,
	projection contextbuilder.Projection,
) (RenderedEnvelope, error) {
	if err := snapshot.Validate(); err != nil {
		return RenderedEnvelope{}, fmt.Errorf("%w: snapshot: %v", ErrInvalidConfig, err)
	}
	if err := projection.Validate(); err != nil {
		return RenderedEnvelope{}, fmt.Errorf("%w: %v", ErrInvalidProjection, err)
	}
	if err := validateProjectionMatch(snapshot.ContextProjection(), projection); err != nil {
		return RenderedEnvelope{}, err
	}
	schema, err := decisionSchemaJSON(snapshot.ActionCatalog())
	if err != nil {
		return RenderedEnvelope{}, err
	}
	envelope := policyEnvelope{
		Schema: EnvelopeSchemaV2, RendererVersion: RendererVersionV2,
		Instruction: decisionInstruction, ActionCatalog: snapshot.ActionCatalog(),
		EvidenceRefs:     snapshot.EvidenceRefs(),
		ProjectedContext: json.RawMessage(projection.Rendered), DecisionSchema: schema,
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return RenderedEnvelope{}, fmt.Errorf("%w: encode envelope: %v", ErrInvalidConfig, err)
	}
	estimatedTokens := estimatePolicyTokens(len(raw))
	if len(raw) > MaxEnvelopeBytes {
		return RenderedEnvelope{}, fmt.Errorf(
			"%w: exact envelope is %d bytes; hard maximum is %d bytes",
			ErrEnvelopeLimit, len(raw), MaxEnvelopeBytes,
		)
	}
	return RenderedEnvelope{
		Version: RendererVersionV2, JSON: string(raw),
		SHA256: policySHA256(string(raw)), Bytes: len(raw),
		EstimatedTokens: estimatedTokens, TokenEstimator: policyTokenEstimator,
	}, nil
}

func validateProjectionLimits(projection contextbuilder.Projection, brain BrainRef) error {
	if projection.RenderedBytes > MaxProjectedContextBytes ||
		projection.RenderedBytes > brain.ContextCeilingBytes ||
		projection.EstimatedTokens > brain.NativeContextLimit {
		return fmt.Errorf(
			"%w: projection is %d bytes/%d estimated tokens; hard maximum is %d bytes and brain limits are %d bytes/%d native units",
			ErrInputLimit, projection.RenderedBytes, projection.EstimatedTokens,
			MaxProjectedContextBytes, brain.ContextCeilingBytes, brain.NativeContextLimit,
		)
	}
	return nil
}

func estimatePolicyTokens(byteCount int) int { return (byteCount + 3) / 4 }

func validateProjectionMatch(
	ref cognition.ContextProjectionRef,
	projection contextbuilder.Projection,
) error {
	if ref.ID != cognition.ContextProjectionID(projection.ID) ||
		ref.SHA256 != projection.RenderedSHA256 ||
		ref.WorkingSetID != cognition.WorkingSetID(projection.WorkingSetID) ||
		ref.WorkingSetVersion != projection.WorkingSetVersion ||
		ref.RendererVersion != projection.RendererVersion {
		return fmt.Errorf("%w: supplied projection does not match the runtime reference", ErrProjectionMismatch)
	}
	return nil
}
