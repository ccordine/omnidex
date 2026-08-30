package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
)

type liveCodingQualificationCall struct {
	kind                                        assemblyline.WorkKind
	jobSHA256, candidateSHA256                  string
	promptSHA256, requestSHA256, responseSHA256 string
	candidate                                   string
	coverageSnapshot                            *liveCodingQualificationCoverageSnapshot
	promptBytes, promptTokens, outputTokens     int
	providerDuration, wallDuration              time.Duration
}

type liveCodingQualificationTransport struct {
	client    *ollama.Client
	selection llm.ProviderIdentitySelection
	expected  llm.ProviderIdentityExpectation
	calls     []liveCodingQualificationCall
}

func newLiveCodingQualificationTransport(
	ctx context.Context,
	client *ollama.Client,
	modelName string,
	contextTokens int,
	discoveryScope string,
) (*liveCodingQualificationTransport, error) {
	if ctx == nil || client == nil {
		return nil, fmt.Errorf("live coding qualification requires context and an exact Ollama client")
	}
	discoveryScope = strings.TrimSpace(discoveryScope)
	if discoveryScope == "" {
		return nil, fmt.Errorf("live coding qualification discovery scope is required")
	}
	selection := llm.ProviderIdentitySelection{
		Model: modelName, NativeContextLimit: contextTokens,
	}
	observed, err := llm.RequireDiscoveredProviderIdentityEvidence(
		ctx, client, selection, discoveryScope,
	)
	if err != nil {
		return nil, fmt.Errorf("discover live qualification provider: %w", err)
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(observed.Evidence, selection)
	if err != nil {
		return nil, fmt.Errorf("derive live qualification provider identity: %w", err)
	}
	return &liveCodingQualificationTransport{
		client: client, selection: selection, expected: expected,
	}, nil
}

func (transport *liveCodingQualificationTransport) execute(
	ctx context.Context,
	job assemblyline.PortableJob,
	modelName string,
) (assemblyline.PortableResult, error) {
	if transport == nil || transport.client == nil || modelName != transport.selection.Model {
		return assemblyline.PortableResult{}, fmt.Errorf("live coding qualification transport authority changed")
	}
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	contract, err := llmResponseContractForPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	if err := validateExactStationStaticCall(prompt, contract, transport.selection); err != nil {
		return assemblyline.PortableResult{}, err
	}
	coverageSnapshot, err := captureLiveCodingQualificationCoverageSnapshot(job)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	gap, err := transport.syntheticGap(job, prompt, contract)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	prepared, err := prepareExactStationCall(gap, contract, modelName, transport.expected, nil)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	requestSHA256, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	started := time.Now()
	generation, generationErr := transport.client.GeneratePreparedExact(ctx, prepared)
	wallDuration := time.Since(started)
	if generationErr != nil {
		return assemblyline.PortableResult{}, generationErr
	}
	generation, err = llm.OwnBoundedPreparedGeneration(generation)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	if err := generation.Validate(); err != nil {
		return assemblyline.PortableResult{}, err
	}
	if generation.ProviderRequestSHA256 != requestSHA256 || generation.ProviderResponseModel != modelName {
		return assemblyline.PortableResult{}, fmt.Errorf("live exact generation identity differs from prepared authority")
	}
	transport.calls = append(transport.calls, liveCodingQualificationCall{
		kind: job.Kind, jobSHA256: job.ID, promptSHA256: qualificationSHA256([]byte(prompt)),
		requestSHA256: requestSHA256, responseSHA256: generation.ProviderResponseSHA256,
		candidateSHA256: qualificationSHA256([]byte(generation.Content)),
		candidate:       generation.Content, coverageSnapshot: coverageSnapshot,
		promptBytes: len(prompt), promptTokens: generation.Usage.PromptEvalCount,
		outputTokens:     generation.Usage.EvalCount,
		providerDuration: time.Duration(generation.Usage.TotalDurationNanos), wallDuration: wallDuration,
	})
	return assemblyline.PortableResult{JobID: job.ID, Candidate: generation.Content}, nil
}

func (transport *liveCodingQualificationTransport) syntheticGap(
	job assemblyline.PortableJob,
	prompt string,
	contract llmResponseContract,
) (queue.StationGapOpening, error) {
	projection, err := exactjson.Canonical(struct {
		Prompt   string `json:"prompt"`
		Renderer string `json:"renderer"`
	}{prompt, assemblyline.PortableRendererV1})
	if err != nil {
		return queue.StationGapOpening{}, err
	}
	stationID, err := queue.StationForPortableJob(job)
	if err != nil {
		return queue.StationGapOpening{}, err
	}
	scope, err := portableModelScope(job.Kind)
	if err != nil {
		return queue.StationGapOpening{}, err
	}
	maxOutputTokens, err := queue.ExpectedPortableStationMaxOutputTokens(
		job, transport.selection.NativeContextLimit,
	)
	if err != nil {
		return queue.StationGapOpening{}, err
	}
	semanticUncertainty, err := assemblyline.SemanticUncertaintyContractForWorkKind(job.Kind)
	if err != nil {
		return queue.StationGapOpening{}, err
	}
	semanticUncertaintySHA256, err := semanticUncertainty.Digest()
	if err != nil {
		return queue.StationGapOpening{}, err
	}
	return queue.StationGapOpening{
		JobID: 1, Generation: 1, StepID: int64(len(transport.calls) + 1), StepAttempt: 1,
		WorkerID: "live-qualification", GapID: job.ID, Station: stationID,
		Scope: scope, PortableSchema: job.Schema,
		WorkID: job.ID, WorkKind: string(job.Kind), RendererVersion: assemblyline.PortableRendererV1,
		Prompt: prompt, ProjectionEnvelope: string(projection),
		ProjectionSHA256:                  qualificationSHA256(projection),
		SemanticUncertaintyContract:       semanticUncertainty,
		SemanticUncertaintyContractSHA256: semanticUncertaintySHA256,
		ContextTokens:                     transport.selection.NativeContextLimit,
		MaxOutputTokens:                   maxOutputTokens, OutputLimitMode: contract.OutputLimitMode,
	}, nil
}

func (transport *liveCodingQualificationTransport) callCount() int {
	return len(transport.calls)
}

func (transport *liveCodingQualificationTransport) callsFrom(start int) []liveCodingQualificationCall {
	return append([]liveCodingQualificationCall(nil), transport.calls[start:]...)
}

func logLiveCodingQualification(
	t *testing.T,
	caseName, modelName, frozenSHA256 string,
	calls []liveCodingQualificationCall,
) {
	t.Helper()
	for index, call := range calls {
		accepted, excluded, exactZero, semanticZero := -1, -1, -1, -1
		if call.coverageSnapshot != nil {
			accepted = len(call.coverageSnapshot.AcceptedRequirements)
			excluded = len(call.coverageSnapshot.ExcludedCandidates)
			exactZero = call.coverageSnapshot.ExactZeroDeltas
			semanticZero = call.coverageSnapshot.SemanticZeroDeltas
		}
		t.Logf(
			"live_coding_qualification case=%s model=%s call=%d kind=%s job_sha256=%s prompt_sha256=%s request_sha256=%s response_sha256=%s candidate_sha256=%s candidate=%q frozen_sha256=%s coverage_accepted=%d coverage_excluded=%d coverage_exact_zero=%d coverage_semantic_zero=%d prompt_bytes=%d prompt_tokens=%d output_tokens=%d provider_ms=%d wall_ms=%d",
			caseName, modelName, index+1, call.kind, call.jobSHA256, call.promptSHA256, call.requestSHA256,
			call.responseSHA256, call.candidateSHA256, call.candidate, frozenSHA256,
			accepted, excluded, exactZero, semanticZero,
			call.promptBytes, call.promptTokens, call.outputTokens,
			call.providerDuration.Milliseconds(), call.wallDuration.Milliseconds(),
		)
	}
}

func qualificationSHA256(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
