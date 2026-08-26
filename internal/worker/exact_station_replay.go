package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

// ExactStationReplay is one read-only execution of a frozen station boundary.
// Generation is retained as exact local benchmark evidence; callers decide
// where to write it and it is never attached to the historical job attempt.
type ExactStationReplay struct {
	SourceOpeningID       int64
	SourceGapOpeningID    int64
	Job                   assemblyline.PortableJob
	Model                 string
	ExpectedIdentity      llm.ProviderIdentityExpectation
	PreparedRequest       string
	PreparedRequestSHA256 string
	Temperature           *llm.ExactPreparedTemperature
	WallDuration          time.Duration
	Generation            llm.PreparedGeneration
	Artifact              ExactStationReplayArtifact
}

// ExactStationReplayArtifact states only what code can verify from a single
// station response. It intentionally does not claim whole-application
// compiler, test, or acceptance success.
type ExactStationReplayArtifact struct {
	Kind            string
	Source          string
	SourceSHA256    string
	StartByte       int
	EndByte         int
	DiscardedBytes  int
	ChangedFromBase bool
}

type exactStationReplayBoundary struct {
	Job      assemblyline.PortableJob
	Prompt   string
	Schema   map[string]any
	Contract llmResponseContract
}

// ReplayExactStation executes the same persisted portable job against one
// selected model. It byte-verifies the stored station boundary before any
// provider request and performs no queue or workspace writes.
func ReplayExactStation(
	ctx context.Context,
	client llm.ExactStationClient,
	point queue.StationCallReplayPoint,
	modelName string,
) (ExactStationReplay, error) {
	result := ExactStationReplay{
		SourceOpeningID: point.Call.ID, SourceGapOpeningID: point.Gap.ID,
		Model: strings.TrimSpace(modelName),
	}
	if ctx == nil || client == nil {
		return result, fmt.Errorf("station replay requires context and exact station client")
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if result.Model == "" {
		return result, fmt.Errorf("station replay model is required")
	}
	boundary, err := validateExactStationReplayPoint(point)
	if err != nil {
		return result, err
	}
	result.Job = boundary.Job
	if err := client.RequireExactPreparedContract(); err != nil {
		return result, fmt.Errorf("station replay provider: %w", err)
	}
	selection, err := providerSelectionForPortableJob(
		boundary.Job, result.Model, point.Gap.ContextTokens,
	)
	if err != nil {
		return result, err
	}
	discoveryScope := fmt.Sprintf("station-replay:%d:%s", point.Call.ID, point.Gap.GapID)
	observed, err := llm.RequireDiscoveredProviderIdentityEvidence(
		ctx, client, selection, discoveryScope,
	)
	if err != nil {
		return result, fmt.Errorf("discover station replay provider: %w", err)
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(observed.Evidence, selection)
	if err != nil {
		return result, fmt.Errorf("derive station replay provider identity: %w", err)
	}
	prepared, err := prepareExactStationCall(point.Gap, boundary.Contract, result.Model, expected, nil)
	if err != nil {
		return result, fmt.Errorf("prepare station replay request: %w", err)
	}
	request, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return result, fmt.Errorf("render station replay request: %w", err)
	}
	result.PreparedRequest, result.PreparedRequestSHA256 = string(request), replaySHA256(string(request))
	result.ExpectedIdentity = expected
	result.Temperature = prepared.Temperature
	return executeExactStationReplayPrepared(
		ctx, client, result, boundary.Job, point.Gap.ContextTokens, prepared,
	)
}

func validateExactStationReplayPoint(
	point queue.StationCallReplayPoint,
) (exactStationReplayBoundary, error) {
	boundary, err := loadStationReplayPortableBoundary(point)
	if err != nil {
		return boundary, err
	}
	call, gap := point.Call, point.Gap
	contract := boundary.Contract
	if gap.Scope != portableModelScope(boundary.Schema) || gap.OutputLimitMode != contract.OutputLimitMode ||
		gap.ContextTokens != gap.MaxOutputTokens ||
		call.ContextTokens != gap.ContextTokens || call.MaxOutputTokens != gap.MaxOutputTokens ||
		call.OutputLimitMode != gap.OutputLimitMode || call.MaxInputTokens != call.ContextTokens ||
		call.ModelInputTokenCeiling != call.ContextTokens || call.Protocol != string(contract.Protocol) {
		return boundary, fmt.Errorf("station replay call limits differ from its frozen natural-output authority")
	}
	modelInput, err := llm.ExactPreparedModelInput(boundary.Prompt, contract.PromptHint)
	if err != nil || call.ModelInput != modelInput || call.ModelInputBytes != len(modelInput) ||
		replaySHA256(modelInput) != call.ModelInputSHA256 {
		return boundary, fmt.Errorf("station replay model input differs from its stored identity")
	}
	selection, err := providerSelectionForPortableJob(boundary.Job, call.Model, call.ContextTokens)
	if err != nil {
		return boundary, err
	}
	if err := validateExactStationStaticCall(boundary.Prompt, boundary.Schema, contract, selection); err != nil {
		return boundary, fmt.Errorf("validate frozen station replay boundary: %w", err)
	}
	return boundary, nil
}

func replayExactStationArtifact(
	job assemblyline.PortableJob,
	raw string,
) (ExactStationReplayArtifact, error) {
	artifact := ExactStationReplayArtifact{
		Kind: "exact_final_response", Source: raw, SourceSHA256: replaySHA256(raw),
		StartByte: 0, EndByte: len(raw),
	}
	switch job.Kind {
	case assemblyline.WorkApplicationContextNeeds:
		artifact.Kind = "application_context_needs"
		var input assemblyline.ApplicationContextNeedInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay application context authority: %w", err)
		}
		if _, err := assemblyline.DecodeApplicationContextNeedDecision(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay application context needs: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationIntent:
		artifact.Kind = "application_intent"
		var input assemblyline.ApplicationIntentInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay application intent authority: %w", err)
		}
		if _, err := assemblyline.DecodeApplicationIntentCandidate(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay application intent: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationProjectStackConstraint:
		artifact.Kind = "application_project_stack_constraint"
		var input assemblyline.ApplicationProjectStackConstraintInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay project stack constraint authority: %w", err)
		}
		decision, err := decodeDirectCodingSemanticJSON[assemblyline.ApplicationProjectStackConstraintDecision](raw)
		if err != nil {
			return artifact, fmt.Errorf("decode replay project stack constraint: %w", err)
		}
		if err := decision.ValidateFor(input); err != nil {
			return artifact, fmt.Errorf("validate replay project stack constraint: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationServiceEndpointRequirement:
		artifact.Kind = "application_service_endpoint_requirement"
		var input assemblyline.ApplicationServiceEndpointRequirementInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint requirement authority: %w", err)
		}
		if _, err := assemblyline.DecodeApplicationServiceEndpointRequirementResult(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint requirement: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationServiceEndpointExposure:
		artifact.Kind = "application_service_endpoint_exposure"
		var input assemblyline.ApplicationServiceEndpointExposureInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint exposure authority: %w", err)
		}
		if _, err := assemblyline.DecodeApplicationServiceEndpointExposureResult(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint exposure: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationServiceEndpointMethod:
		artifact.Kind = "application_service_endpoint_method"
		var input assemblyline.ApplicationServiceEndpointMethodInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint method authority: %w", err)
		}
		if _, err := assemblyline.DecodeApplicationServiceEndpointMethodResult(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint method: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationServiceEndpointRouteTemplate:
		artifact.Kind = "application_service_endpoint_route_template"
		var input assemblyline.ApplicationServiceEndpointRouteTemplateInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint route authority: %w", err)
		}
		if _, err := assemblyline.DecodeApplicationServiceEndpointRouteTemplateResult(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint route: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationServiceEndpointRequestMedia:
		artifact.Kind = "application_service_endpoint_request_media"
		var input assemblyline.ApplicationServiceEndpointRequestMediaInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint request-media authority: %w", err)
		}
		if _, err := assemblyline.DecodeApplicationServiceEndpointRequestMediaResult(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint request media: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationServiceEndpointResponseMedia:
		artifact.Kind = "application_service_endpoint_response_media"
		var input assemblyline.ApplicationServiceEndpointResponseMediaInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint response-media authority: %w", err)
		}
		if _, err := assemblyline.DecodeApplicationServiceEndpointResponseMediaResult(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint response media: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationServiceEndpointSuccessStatus:
		artifact.Kind = "application_service_endpoint_success_status"
		var input assemblyline.ApplicationServiceEndpointSuccessStatusInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint success-status authority: %w", err)
		}
		if _, err := assemblyline.DecodeApplicationServiceEndpointSuccessStatusResult(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay service endpoint success status: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationJobSpecification:
		artifact.Kind = "application_job_specification"
		if _, err := assemblyline.DecodeApplicationJobSpecificationResult(job, raw); err != nil {
			return artifact, fmt.Errorf("decode replay application job specification: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationTargetTree:
		artifact.Kind = "application_target_tree"
		var input assemblyline.TargetTreeInput
		if err := json.Unmarshal(job.Payload, &input); err != nil {
			return artifact, fmt.Errorf("decode replay target tree authority: %w", err)
		}
		if _, err := assemblyline.DecodeTargetTreeCandidate(input, raw); err != nil {
			return artifact, fmt.Errorf("decode replay target tree: %w", err)
		}
		return artifact, nil
	}
	if job.Kind == assemblyline.WorkTypeScriptRepairGuidance {
		guidance, err := assemblyline.DecodeTypeScriptRepairGuidanceResult(job, raw)
		artifact.Kind = "typescript_repair_guidance"
		if err != nil {
			return artifact, err
		}
		artifact.Source = guidance.Instruction
		artifact.SourceSHA256 = replaySHA256(guidance.Instruction)
		artifact.StartByte, artifact.EndByte = 0, len(guidance.Instruction)
		artifact.DiscardedBytes = len(raw) - len(guidance.Instruction)
		return artifact, nil
	}
	if job.Kind != assemblyline.WorkFragmentGeneration && job.Kind != assemblyline.WorkFragmentCorrection {
		return artifact, nil
	}
	var correction assemblyline.FragmentCorrectionInput
	var signature, language, current string
	var region *assemblyline.TypeScriptFragmentRepairRegion
	if job.Kind == assemblyline.WorkFragmentGeneration {
		var generation assemblyline.FragmentGenerationInput
		if err := json.Unmarshal(job.Payload, &generation); err != nil {
			return artifact, fmt.Errorf("decode replay fragment generation input: %w", err)
		}
		signature, language = generation.Signature, generation.Language
	} else {
		if err := json.Unmarshal(job.Payload, &correction); err != nil {
			return artifact, fmt.Errorf("decode replay fragment correction input: %w", err)
		}
		signature, language, current, region = correction.Signature, correction.Language,
			correction.CurrentDeclaration, correction.RepairRegion
	}
	if language != "typescript" {
		return artifact, nil
	}
	if region != nil {
		replacement, err := assemblyline.ProjectTypeScriptFragmentRepairResponse(*region, raw)
		artifact.Kind, artifact.Source = "typescript_repair_region", replacement
		artifact.SourceSHA256 = replaySHA256(replacement)
		artifact.StartByte, artifact.EndByte = 0, len(replacement)
		artifact.DiscardedBytes = len(raw) - len(replacement)
		artifact.ChangedFromBase = replacement != region.Source
		if err != nil {
			return artifact, fmt.Errorf("project replay TypeScript repair region: %w", err)
		}
		return artifact, nil
	}
	projection, err := assemblyline.ProjectTypeScriptFunctionModelResponse(
		assemblyline.TypeScriptFunctionContract{Signature: signature, TSX: true}, raw,
	)
	artifact.Kind, artifact.Source, artifact.SourceSHA256 = "typescript_function", projection.Source, projection.SourceSHA256
	artifact.StartByte, artifact.EndByte, artifact.DiscardedBytes = projection.StartByte, projection.EndByte, projection.DiscardedBytes
	artifact.ChangedFromBase = current != "" && projection.Source != current
	if err != nil {
		return artifact, fmt.Errorf("project replay TypeScript function: %w", err)
	}
	return artifact, nil
}

func replaySHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
