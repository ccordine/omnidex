package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	jobSHA256                                   string
	promptSHA256, requestSHA256, responseSHA256 string
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
) (*liveCodingQualificationTransport, error) {
	if ctx == nil || client == nil {
		return nil, fmt.Errorf("live coding qualification requires context and an exact Ollama client")
	}
	selection := llm.ProviderIdentitySelection{
		Model: modelName, NativeContextLimit: contextTokens,
	}
	observed, err := llm.RequireDiscoveredProviderIdentityEvidence(
		ctx, client, selection, "live-coding-requirements-workload-qualification-v1",
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
	prompt, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	contract, err := llmResponseContractForScope(portableModelScope(schema))
	if err != nil {
		return assemblyline.PortableResult{}, err
	}
	if err := validateExactStationStaticCall(prompt, schema, contract, transport.selection); err != nil {
		return assemblyline.PortableResult{}, err
	}
	gap, err := transport.syntheticGap(job, prompt, schema, contract)
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
		promptBytes: len(prompt), promptTokens: generation.Usage.PromptEvalCount,
		outputTokens:     generation.Usage.EvalCount,
		providerDuration: time.Duration(generation.Usage.TotalDurationNanos), wallDuration: wallDuration,
	})
	return assemblyline.PortableResult{JobID: job.ID, Candidate: generation.Content}, nil
}

func (transport *liveCodingQualificationTransport) syntheticGap(
	job assemblyline.PortableJob,
	prompt string,
	schema map[string]any,
	contract llmResponseContract,
) (queue.StationGapOpening, error) {
	schemaJSON, err := exactjson.Canonical(schema)
	if err != nil {
		return queue.StationGapOpening{}, err
	}
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{prompt, assemblyline.PortableRendererV3, schemaJSON})
	if err != nil {
		return queue.StationGapOpening{}, err
	}
	stationID, err := queue.StationForPortableJob(job)
	if err != nil {
		return queue.StationGapOpening{}, err
	}
	return queue.StationGapOpening{
		JobID: 1, Generation: 1, StepID: int64(len(transport.calls) + 1), StepAttempt: 1,
		WorkerID: "live-qualification", GapID: job.ID, Station: stationID,
		Scope: portableModelScope(schema), PortableSchema: job.Schema,
		WorkID: job.ID, WorkKind: string(job.Kind), RendererVersion: assemblyline.PortableRendererV3,
		Prompt: prompt, ResponseSchema: schemaJSON, ProjectionEnvelope: string(projection),
		ProjectionSHA256: qualificationSHA256(projection), ContextTokens: transport.selection.NativeContextLimit,
		MaxOutputTokens: transport.selection.NativeContextLimit, OutputLimitMode: contract.OutputLimitMode,
	}, nil
}

func (transport *liveCodingQualificationTransport) callCount() int {
	return len(transport.calls)
}

func (transport *liveCodingQualificationTransport) callsFrom(start int) []liveCodingQualificationCall {
	return append([]liveCodingQualificationCall(nil), transport.calls[start:]...)
}

func assertLiveCodingQualificationCalls(t *testing.T, calls []liveCodingQualificationCall, featureCount int) {
	t.Helper()
	counts := map[assemblyline.WorkKind]int{}
	initialTaskJobs := map[string]struct{}{}
	for _, call := range calls {
		counts[call.kind]++
		for _, digest := range []string{call.jobSHA256, call.promptSHA256, call.requestSHA256, call.responseSHA256} {
			if decoded, err := hex.DecodeString(digest); err != nil || len(decoded) != sha256.Size {
				t.Fatal("live qualification call lacks an exact digest")
			}
		}
		if call.kind == assemblyline.WorkApplicationJobSpecification {
			if _, duplicate := initialTaskJobs[call.jobSHA256]; duplicate {
				t.Fatal("live qualification repeated one initial workload leaf")
			}
			initialTaskJobs[call.jobSHA256] = struct{}{}
		}
		if call.promptBytes < 1 || call.promptTokens < 1 || call.outputTokens < 1 ||
			call.providerDuration <= 0 || call.wallDuration <= 0 {
			t.Fatal("live qualification call lacks bounded native metrics")
		}
	}
	repairs := counts[assemblyline.WorkApplicationJobSpecificationRepair]
	if counts[assemblyline.WorkApplicationRequirements] != 1 ||
		counts[assemblyline.WorkApplicationJobSpecification] != featureCount ||
		counts[assemblyline.WorkApplicationJobSpecificationReview] != featureCount+repairs ||
		repairs > featureCount*2 {
		t.Fatalf("live qualification call shape differs from one specification plus mandatory review and bounded repair per feature: %v", counts)
	}
}

func logLiveCodingQualification(
	t *testing.T,
	caseName, modelName, frozenSHA256 string,
	calls []liveCodingQualificationCall,
) {
	t.Helper()
	for index, call := range calls {
		t.Logf(
			"live_coding_qualification case=%s model=%s call=%d kind=%s job_sha256=%s prompt_sha256=%s request_sha256=%s response_sha256=%s frozen_sha256=%s prompt_bytes=%d prompt_tokens=%d output_tokens=%d provider_ms=%d wall_ms=%d",
			caseName, modelName, index+1, call.kind, call.jobSHA256, call.promptSHA256, call.requestSHA256,
			call.responseSHA256, frozenSHA256, call.promptBytes, call.promptTokens, call.outputTokens,
			call.providerDuration.Milliseconds(), call.wallDuration.Milliseconds(),
		)
	}
}

func qualificationSHA256(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
