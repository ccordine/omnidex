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
	"github.com/gryph/omnidex/internal/exactjson"
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
	discoveryScope := fmt.Sprintf(
		"station-replay:%d:%s:semantic-uncertainty:%s",
		point.Call.ID, point.Gap.GapID, point.Gap.SemanticUncertaintyContractSHA256,
	)
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
		ctx, client, result, boundary.Job, point.Gap.RendererVersion,
		point.Gap.ContextTokens, prepared,
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
	expectedScope, err := portableModelScope(boundary.Job.Kind)
	if err != nil {
		return boundary, err
	}
	expectedMaxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(
		boundary.Job, gap.ContextTokens,
	)
	if err != nil {
		return boundary, err
	}
	if gap.Scope != expectedScope || gap.OutputLimitMode != contract.OutputLimitMode ||
		gap.MaxOutputTokens != expectedMaxOutputTokens ||
		call.ContextTokens != gap.ContextTokens || call.MaxOutputTokens != gap.MaxOutputTokens ||
		call.OutputLimitMode != gap.OutputLimitMode || call.MaxInputTokens != call.ContextTokens ||
		call.ModelInputTokenCeiling != call.ContextTokens || call.Protocol != string(contract.Protocol) {
		return boundary, fmt.Errorf("station replay call limits differ from its frozen natural-output authority")
	}
	modelInput, err := exactStationReplayStoredModelInput(boundary, gap, call)
	if err != nil || call.ModelInput != modelInput || call.ModelInputBytes != len(modelInput) ||
		replaySHA256(modelInput) != call.ModelInputSHA256 {
		return boundary, fmt.Errorf("station replay model input differs from its stored identity")
	}
	selection, err := providerSelectionForPortableJob(boundary.Job, call.Model, call.ContextTokens)
	if err != nil {
		return boundary, err
	}
	if err := validateExactStationStaticCall(boundary.Prompt, contract, selection); err != nil {
		return boundary, fmt.Errorf("validate frozen station replay boundary: %w", err)
	}
	return boundary, nil
}

func exactStationReplayStoredModelInput(
	boundary exactStationReplayBoundary,
	gap queue.StationGapOpening,
	call queue.StationCallOpening,
) (string, error) {
	var expected llm.ProviderIdentityExpectation
	if err := exactjson.ValidateObject(
		call.Expectation, expected, "station replay provider expectation",
	); err != nil {
		return "", err
	}
	if err := json.Unmarshal(call.Expectation, &expected); err != nil {
		return "", fmt.Errorf("decode station replay provider expectation: %w", err)
	}
	canonical, err := exactjson.Canonical(expected)
	if err != nil {
		return "", err
	}
	if string(canonical) != string(call.Expectation) ||
		replaySHA256(string(canonical)) != call.ExpectationSHA256 ||
		expected.Model != call.Model || expected.TokenizerProfile != call.TokenizerProfile ||
		expected.NativeContextLimit != call.ContextTokens {
		return "", fmt.Errorf("station replay provider expectation differs from its stored identity")
	}
	stop, err := queue.ExpectedStationCallStopSequence(gap, expected)
	if err != nil {
		return "", err
	}
	return llm.ExactPreparedRequestModelInput(llm.PreparedModel{
		BaseModel: call.Model, ContextModel: call.Model,
		Prompt: boundary.Prompt, PromptHint: boundary.Contract.PromptHint,
		ContextTokens:               call.ContextTokens,
		RawTextStopSequence:         stop,
		ProviderIdentityExpectation: &expected,
	})
}

func replayExactStationArtifact(
	job assemblyline.PortableJob,
	raw string,
) (ExactStationReplayArtifact, error) {
	return replayExactStationArtifactForRenderer(
		job, assemblyline.PortableRendererV8, raw,
	)
}

func replayExactStationArtifactForRenderer(
	job assemblyline.PortableJob,
	renderer string,
	raw string,
) (ExactStationReplayArtifact, error) {
	artifact := ExactStationReplayArtifact{
		Kind: "exact_final_response", Source: raw, SourceSHA256: replaySHA256(raw),
		StartByte: 0, EndByte: len(raw),
	}
	switch job.Kind {
	case assemblyline.WorkApplicationRequirementCoverage:
		artifact.Kind = string(job.Kind)
		if _, err := assemblyline.DecodeApplicationRequirementCoverageLeafForPortableRenderer(
			job.Payload, renderer, raw,
		); err != nil {
			return artifact, fmt.Errorf("decode replay application requirement coverage: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationRequirement:
		artifact.Kind = string(job.Kind)
		if _, err := assemblyline.DecodeApplicationRequirementLeafForPortableRenderer(
			job.Payload, renderer, raw,
		); err != nil {
			return artifact, fmt.Errorf("decode replay application requirement: %w", err)
		}
		return artifact, nil
	case assemblyline.WorkApplicationProjectStackConstraint:
		artifact.Kind = string(job.Kind)
		if _, err := assemblyline.DecodeApplicationProjectStackConstraintDecisionForPortableRenderer(
			job.Payload, renderer, raw,
		); err != nil {
			return artifact, fmt.Errorf("decode replay project stack constraint: %w", err)
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
	if semanticArtifact, handled, err := replayExactStationSemanticArtifact(job, raw, artifact); handled {
		return semanticArtifact, err
	}
	return replayExactStationSourceArtifact(job, raw, artifact)
}

func replaySHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
