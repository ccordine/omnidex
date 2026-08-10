package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	ProjectionTraceSchemaV1 = "omnidex.cognition-projection-trace.v1"
	ModelCallTraceSchemaV1  = "omnidex.cognition-model-call-trace.v1"
)

type StationBudget struct {
	MaxInputBytes   int `json:"max_input_bytes"`
	MaxInputTokens  int `json:"max_input_tokens"`
	MaxOutputBytes  int `json:"max_output_bytes"`
	MaxOutputTokens int `json:"max_output_tokens"`
}

type ProjectionReferenceIdentity struct {
	URI           string `json:"uri"`
	Version       string `json:"version"`
	ContentSHA256 string `json:"content_sha256"`
}

type ProjectedReference struct {
	Ref           ProjectionReferenceIdentity   `json:"ref"`
	SourceRefs    []ProjectionReferenceIdentity `json:"source_refs"`
	RenderedBytes int64                         `json:"rendered_bytes"`
}

type ProjectionTrace struct {
	Schema           string               `json:"schema"`
	ProjectionID     string               `json:"projection_id"`
	ProjectionSHA256 string               `json:"projection_sha256"`
	RenderedBytes    int64                `json:"rendered_bytes"`
	EstimatedTokens  int64                `json:"estimated_tokens"`
	TokenEstimator   string               `json:"token_estimator"`
	Selected         []ProjectedReference `json:"selected"`
}

type ModelCallTrace struct {
	Schema           string        `json:"schema"`
	ProjectionID     string        `json:"projection_id"`
	ProjectionSHA256 string        `json:"projection_sha256"`
	Budget           StationBudget `json:"budget"`
	InputBytes       int64         `json:"input_bytes"`
	InputTokens      int64         `json:"input_tokens"`
	OutputBytes      int64         `json:"output_bytes"`
	OutputTokens     int64         `json:"output_tokens"`
}

func (budget StationBudget) Validate() error {
	if budget.MaxInputBytes <= 0 || budget.MaxInputBytes > cognition.MaxPolicyInputBytes ||
		budget.MaxInputTokens <= 0 || budget.MaxInputTokens > cognition.MaxPolicyInputTokens ||
		budget.MaxOutputBytes <= 0 || budget.MaxOutputBytes > cognition.MaxPolicyOutputBytes ||
		budget.MaxOutputTokens <= 0 || budget.MaxOutputTokens > cognition.MaxPolicyOutputTokens {
		return fmt.Errorf("station call budget is outside registered bounds")
	}
	return nil
}

func (ref ProjectionReferenceIdentity) Validate() error {
	if err := requireExact(ref.URI, "projected reference URI", 1024); err != nil {
		return err
	}
	if err := requireExact(ref.Version, "projected reference version", 256); err != nil {
		return err
	}
	if !validDigest(ref.ContentSHA256) {
		return fmt.Errorf("projected reference content digest is invalid")
	}
	return nil
}

func (projection ProjectionTrace) Validate() error {
	if projection.Schema != ProjectionTraceSchemaV1 ||
		requireExact(projection.ProjectionID, "projection trace ID", 512) != nil ||
		!validDigest(projection.ProjectionSHA256) || projection.RenderedBytes <= 0 ||
		projection.RenderedBytes > cognition.MaxPolicyInputBytes || projection.EstimatedTokens <= 0 ||
		projection.EstimatedTokens > cognition.MaxPolicyInputTokens ||
		requireExact(projection.TokenEstimator, "projection token estimator", 256) != nil ||
		projection.Selected == nil || len(projection.Selected) > cognition.MaxEvidenceRefs {
		return fmt.Errorf("Context Projection trace authority is invalid")
	}
	seen := make(map[ProjectionReferenceIdentity]struct{}, len(projection.Selected))
	var selectedBytes int64
	for index, selected := range projection.Selected {
		if err := selected.Ref.Validate(); err != nil {
			return fmt.Errorf("projected reference %d: %w", index+1, err)
		}
		if selected.RenderedBytes <= 0 || selected.RenderedBytes > projection.RenderedBytes {
			return fmt.Errorf("projected reference %d byte count is invalid", index+1)
		}
		if selected.SourceRefs == nil || len(selected.SourceRefs) > cognition.MaxEvidenceRefs {
			return fmt.Errorf("projected reference %d source lineage is not explicit and bounded", index+1)
		}
		seenSources := make(map[ProjectionReferenceIdentity]struct{}, len(selected.SourceRefs))
		for sourceIndex, source := range selected.SourceRefs {
			if err := source.Validate(); err != nil {
				return fmt.Errorf("projected reference %d source %d: %w", index+1, sourceIndex+1, err)
			}
			if source == selected.Ref {
				return fmt.Errorf("projected reference %d cites itself as source", index+1)
			}
			if _, duplicate := seenSources[source]; duplicate {
				return fmt.Errorf("projected reference %d source %d is duplicated", index+1, sourceIndex+1)
			}
			seenSources[source] = struct{}{}
		}
		if _, duplicate := seen[selected.Ref]; duplicate {
			return fmt.Errorf("projected reference %d is duplicated", index+1)
		}
		seen[selected.Ref] = struct{}{}
		selectedBytes += selected.RenderedBytes
	}
	if selectedBytes > projection.RenderedBytes {
		return fmt.Errorf("projected reference bytes exceed rendered projection bytes")
	}
	return nil
}

func (call ModelCallTrace) Validate() error {
	if call.Schema != ModelCallTraceSchemaV1 ||
		requireExact(call.ProjectionID, "model-call projection ID", 512) != nil ||
		!validDigest(call.ProjectionSHA256) {
		return fmt.Errorf("model-call trace authority is invalid")
	}
	if err := call.Budget.Validate(); err != nil {
		return err
	}
	if call.InputBytes <= 0 || call.InputTokens <= 0 || call.OutputBytes < 0 || call.OutputTokens < 0 ||
		call.InputBytes > int64(call.Budget.MaxInputBytes) ||
		call.InputTokens > int64(call.Budget.MaxInputTokens) ||
		call.OutputBytes > int64(call.Budget.MaxOutputBytes) ||
		call.OutputTokens > int64(call.Budget.MaxOutputTokens) ||
		(call.OutputBytes == 0) != (call.OutputTokens == 0) {
		return fmt.Errorf("model-call trace usage exceeds its station budget")
	}
	return nil
}

func decodeTracePayload(payload taskstate.JSONObject, target any, label string) error {
	return decodeStrictJSON(payload.Bytes(), target, label)
}
