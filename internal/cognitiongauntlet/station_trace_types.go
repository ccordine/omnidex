package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	ProjectionTraceSchemaV1   = "omnidex.cognition-projection-trace.v1"
	ModelCallTraceSchemaV4    = "omnidex.cognition-model-call-trace.v4"
	PolicyDispositionSchemaV3 = "omnidex.cognition-policy-disposition-trace.v3"
)

type PolicyCallDisposition string

const (
	PolicyCallResultDisposition    PolicyCallDisposition = "terminal_result"
	PolicyCallAbandonedDisposition PolicyCallDisposition = "abandoned_without_result"
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
	Schema                      string                           `json:"schema"`
	ProjectionID                string                           `json:"projection_id"`
	ProjectionSHA256            string                           `json:"projection_sha256"`
	Budget                      StationBudget                    `json:"budget"`
	ResultStatus                cognitionpolicy.CallResultStatus `json:"result_status"`
	FailureCode                 cognitionpolicy.CallFailureCode  `json:"failure_code,omitempty"`
	ProviderResponseDisposition llm.ProviderResponseDisposition  `json:"provider_response_disposition"`
	ProviderRequestDisposition  llm.ProviderRequestDisposition   `json:"provider_request_disposition"`
	ProviderDoneReason          string                           `json:"provider_done_reason"`
	ProviderUsagePresent        bool                             `json:"provider_usage_present"`
	ProviderUsage               llm.ProviderGenerationUsage      `json:"provider_usage"`
	InputBytes                  int64                            `json:"input_bytes"`
	InputTokens                 int64                            `json:"input_tokens"`
	OutputBytes                 int64                            `json:"output_bytes"`
	OutputTokens                int64                            `json:"output_tokens"`
}

type PolicyDispositionTrace struct {
	Schema                     string                           `json:"schema"`
	Disposition                PolicyCallDisposition            `json:"disposition"`
	ProjectionID               string                           `json:"projection_id"`
	ProjectionSHA256           string                           `json:"projection_sha256"`
	Budget                     StationBudget                    `json:"budget"`
	ResultStatus               cognitionpolicy.CallResultStatus `json:"result_status"`
	FailureCode                cognitionpolicy.CallFailureCode  `json:"failure_code"`
	ProviderRequestDisposition llm.ProviderRequestDisposition   `json:"provider_request_disposition"`
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
	if call.Schema != ModelCallTraceSchemaV4 ||
		requireExact(call.ProjectionID, "model-call projection ID", 512) != nil ||
		!validDigest(call.ProjectionSHA256) ||
		call.ProviderRequestDisposition != llm.ProviderRequestDispatched {
		return fmt.Errorf("model-call trace authority is invalid")
	}
	if err := call.Budget.Validate(); err != nil {
		return err
	}
	return validateExecutedCallUsage(call)
}

func (trace PolicyDispositionTrace) Validate() error {
	if trace.Schema != PolicyDispositionSchemaV3 ||
		requireExact(trace.ProjectionID, "policy disposition projection ID", 512) != nil ||
		!validDigest(trace.ProjectionSHA256) {
		return fmt.Errorf("non-inference policy disposition authority is invalid")
	}
	if err := trace.Budget.Validate(); err != nil {
		return err
	}
	switch trace.Disposition {
	case PolicyCallResultDisposition:
		if (trace.ProviderRequestDisposition != llm.ProviderRequestNotDispatched &&
			trace.ProviderRequestDisposition != llm.ProviderRequestWriteIndeterminate) ||
			trace.ResultStatus != cognitionpolicy.CallResultFailed ||
			!registeredPolicyFailureCode(trace.FailureCode) {
			return fmt.Errorf("non-inference policy result disposition is invalid")
		}
	case PolicyCallAbandonedDisposition:
		if trace.ProviderRequestDisposition != "" || trace.ResultStatus != "" ||
			trace.FailureCode != "" {
			return fmt.Errorf("abandoned policy disposition invents a terminal provider result")
		}
	default:
		return fmt.Errorf("policy disposition is not registered")
	}
	return nil
}

func registeredPolicyFailureCode(code cognitionpolicy.CallFailureCode) bool {
	switch code {
	case cognitionpolicy.CallFailureGeneration,
		cognitionpolicy.CallFailureProviderIdentity,
		cognitionpolicy.CallFailureProviderRequest,
		cognitionpolicy.CallFailurePolicyAuthority,
		cognitionpolicy.CallFailureProviderEvidence:
		return true
	default:
		return false
	}
}

func decodeTracePayload(payload taskstate.JSONObject, target any, label string) error {
	return decodeStrictJSON(payload.Bytes(), target, label)
}
