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
	selection := llm.ProviderIdentitySelection{
		Model: result.Model, NativeContextLimit: point.Gap.ContextTokens,
	}
	if err := selection.Validate(); err != nil {
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
	prepared, err := prepareExactStationCall(point.Gap, boundary.Contract, result.Model, expected)
	if err != nil {
		return result, fmt.Errorf("prepare station replay request: %w", err)
	}
	request, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return result, fmt.Errorf("render station replay request: %w", err)
	}
	result.PreparedRequest, result.PreparedRequestSHA256 = string(request), replaySHA256(string(request))
	result.ExpectedIdentity = expected

	started := time.Now()
	generation, generationErr := client.GeneratePreparedExact(ctx, prepared)
	result.WallDuration = time.Since(started)
	owned, ownershipErr := llm.OwnBoundedPreparedGeneration(generation)
	if ownershipErr != nil {
		return result, fmt.Errorf("own station replay generation: %w", ownershipErr)
	}
	result.Generation = owned
	if generationErr != nil {
		return result, fmt.Errorf("generate station replay: %w", generationErr)
	}
	if err := owned.Validate(); err != nil {
		return result, fmt.Errorf("validate station replay generation: %w", err)
	}
	if owned.ProviderRequestSHA256 != result.PreparedRequestSHA256 || owned.ProviderResponseModel != result.Model {
		return result, fmt.Errorf("station replay generation differs from its prepared authority")
	}
	derived, err := llm.DeriveExactProviderIdentityExpectation(owned.ProviderIdentityEvidence, selection)
	if err != nil || derived != expected {
		return result, fmt.Errorf("station replay provider identity differs from discovery authority")
	}
	if err := llm.ValidateExactPreparedNaturalUsage(point.Gap.ContextTokens, owned.Usage); err != nil {
		return result, fmt.Errorf("validate station replay native usage: %w", err)
	}
	projection, err := assemblyline.NewExactPortableResultProjection(owned.Content)
	if err != nil {
		return result, fmt.Errorf("project station replay response: %w", err)
	}
	if err := (assemblyline.PortableResult{
		JobID: boundary.Job.ID, Candidate: owned.Content, Projection: &projection,
	}).ValidateFor(boundary.Job); err != nil {
		return result, fmt.Errorf("validate station replay portable response: %w", err)
	}
	artifact, err := replayExactStationArtifact(boundary.Job, owned.Content)
	result.Artifact = artifact
	if err != nil {
		return result, err
	}
	return result, nil
}

func validateExactStationReplayPoint(
	point queue.StationCallReplayPoint,
) (exactStationReplayBoundary, error) {
	var boundary exactStationReplayBoundary
	call, gap := point.Call, point.Gap
	if call.ID < 1 || gap.ID < 1 || call.GapOpeningID != gap.ID || call.JobID != gap.JobID ||
		call.Generation != gap.Generation || call.StepID != gap.StepID ||
		call.StepAttempt != gap.StepAttempt || call.WorkerID != gap.WorkerID || call.GapID != gap.GapID {
		return boundary, fmt.Errorf("station replay point does not preserve one exact call and gap authority")
	}
	if gap.RendererVersion != assemblyline.PortableRendererV3 {
		return boundary, fmt.Errorf("station replay renderer %q is not current", gap.RendererVersion)
	}
	if err := exactjson.ValidateObject(
		[]byte(gap.PortableEnvelope), assemblyline.PortableJob{}, "station replay portable envelope",
	); err != nil {
		return boundary, fmt.Errorf("validate station replay portable envelope: %w", err)
	}
	boundary.Job = assemblyline.PortableJob{
		Schema: gap.PortableSchema, ID: gap.WorkID, Kind: assemblyline.WorkKind(gap.WorkKind),
		Payload: append(json.RawMessage(nil), gap.PortablePayload...),
	}
	if err := boundary.Job.Validate(); err != nil {
		return boundary, fmt.Errorf("validate station replay portable job: %w", err)
	}
	envelope, err := exactjson.Canonical(boundary.Job)
	if err != nil || string(envelope) != gap.PortableEnvelope || replaySHA256(string(envelope)) != gap.PortableEnvelopeSHA256 {
		return boundary, fmt.Errorf("station replay portable envelope differs from its stored identity")
	}
	if boundary.Job.Schema != gap.PortableSchema || boundary.Job.ID != gap.WorkID ||
		string(boundary.Job.Kind) != gap.WorkKind || string(boundary.Job.Payload) != gap.PortablePayload ||
		replaySHA256(string(boundary.Job.Payload)) != gap.PortablePayloadSHA256 {
		return boundary, fmt.Errorf("station replay portable fields differ from their stored identity")
	}
	prompt := gap.Prompt
	if strings.TrimSpace(prompt) == "" {
		return boundary, fmt.Errorf("station replay stored prompt is empty")
	}
	var schema map[string]any
	if string(gap.ResponseSchema) != "null" {
		if err := json.Unmarshal(gap.ResponseSchema, &schema); err != nil {
			return boundary, fmt.Errorf("decode station replay response schema: %w", err)
		}
	}
	schemaRaw, err := exactjson.Canonical(schema)
	if err != nil || string(schemaRaw) != string(gap.ResponseSchema) {
		return boundary, fmt.Errorf("station replay response schema differs from its stored canonical identity")
	}
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{prompt, assemblyline.PortableRendererV3, schemaRaw})
	if err != nil || string(projection) != gap.ProjectionEnvelope ||
		replaySHA256(string(projection)) != gap.ProjectionSHA256 {
		return boundary, fmt.Errorf("station replay projection differs from its stored identity")
	}
	contract, err := llmResponseContractForPortableJob(boundary.Job, schema)
	if err != nil {
		return boundary, err
	}
	if gap.Scope != portableModelScope(schema) || gap.OutputLimitMode != contract.OutputLimitMode ||
		gap.ContextTokens != gap.MaxOutputTokens ||
		call.ContextTokens != gap.ContextTokens || call.MaxOutputTokens != gap.MaxOutputTokens ||
		call.OutputLimitMode != gap.OutputLimitMode || call.MaxInputTokens != call.ContextTokens ||
		call.ModelInputTokenCeiling != call.ContextTokens || call.Protocol != string(contract.Protocol) {
		return boundary, fmt.Errorf("station replay call limits differ from its frozen natural-output authority")
	}
	modelInput, err := llm.ExactPreparedModelInput(prompt, contract.PromptHint)
	if err != nil || call.ModelInput != modelInput || call.ModelInputBytes != len(modelInput) ||
		replaySHA256(modelInput) != call.ModelInputSHA256 {
		return boundary, fmt.Errorf("station replay model input differs from its stored identity")
	}
	if err := validateExactStationStaticCall(prompt, schema, contract, llm.ProviderIdentitySelection{
		Model: call.Model, NativeContextLimit: call.ContextTokens,
	}); err != nil {
		return boundary, fmt.Errorf("validate frozen station replay boundary: %w", err)
	}
	boundary.Prompt, boundary.Schema, boundary.Contract = prompt, schema, contract
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
