package worker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type exactEvidenceStationFixture struct {
	candidate    string
	doneReason   string
	promptTokens int
	outputTokens int
	partial      []byte
	entered      chan<- struct{}
	release      <-chan struct{}
}

type exactEvidenceStationClient struct {
	fixtures []exactEvidenceStationFixture
	prepared []llm.PreparedModel
	calls    int
}

func TestTerminalizedLLMOutcomeRelinquishesLostExecutionAuthority(t *testing.T) {
	t.Parallel()
	service := &Service{}
	claim := &model.ClaimedStep{Authority: model.StepAttemptAuthority{
		JobID: 1, Generation: 1, StepID: 2, Attempt: 1, WorkerID: "worker",
	}}
	if !service.skipFailureForLostExecutionAuthority(
		"worker", claim, fmt.Errorf("finalize station: %w", queue.ErrLLMCallTerminalizedByAttempt),
	) {
		t.Fatal("terminalized LLM outcome was treated as a new step failure")
	}
}

func (client *exactEvidenceStationClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	if client.calls >= len(client.fixtures) {
		return llm.PreparedGeneration{}, fmt.Errorf("unexpected provider invocation")
	}
	fixture := client.fixtures[client.calls]
	client.calls++
	client.prepared = append(client.prepared, prepared)
	if fixture.entered != nil {
		close(fixture.entered)
	}
	if fixture.release != nil {
		<-fixture.release
	}
	if fixture.partial != nil {
		return exactEvidencePartialGeneration(prepared, fixture.partial), errors.New(
			"provider body read stopped after the captured prefix",
		)
	}
	if fixture.doneReason == "length" {
		return exactEvidenceLengthGeneration(
			prepared, fixture.candidate, fixture.promptTokens, fixture.outputTokens,
		)
	}
	return exactEvidenceSuccessfulGeneration(prepared, fixture.candidate)
}

func TestFreshSchemaOutputLimitFailsOnceWithoutPartialConsumption(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for output-limit evidence coverage")
	}
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise terminal output limit", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "output-continuation-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{
			candidate:  "unfinished output that must never reach the semantic decoder",
			doneReason: "length", promptTokens: 11,
		},
	}}
	service := &Service{
		repo: repository, stationClient: client, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service, ctx: ctx, claim: claim,
	}, "output-continuation")
	classification, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	decodedCandidates := make([]string, 0, 1)
	_, err = runDirectCodingSemanticLeafCall(
		runtime, "fixture-model", "classification", classification, nil,
		func(candidate string) (string, error) {
			decodedCandidates = append(decodedCandidates, candidate)
			decoded, err := assemblyline.DecodeApplicationClassification(
				assemblyline.ApplicationClassificationInput{
					UserRequest: "classify one interface",
				},
				candidate,
			)
			if err != nil {
				return "", err
			}
			return string(decoded.Surface), nil
		},
	)
	if err == nil {
		t.Fatal("output-limit response was accepted")
	}
	var limit *llm.ExactPreparedOutputLimitReachedError
	if !errors.As(err, &limit) {
		t.Fatalf("output-limit error=%v", err)
	}
	if client.calls != 1 || runtime.ProviderCalls == nil || runtime.ProviderCalls() != 1 {
		t.Fatalf("provider calls=%d runtime=%v", client.calls, runtime.ProviderCalls())
	}
	if len(decodedCandidates) != 0 {
		t.Fatalf("semantic decoder candidates=%q", decodedCandidates)
	}
	if len(client.prepared) != 1 {
		t.Fatalf("prepared calls=%d", len(client.prepared))
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Outcome == nil || calls[0].Iteration != 1 ||
		calls[0].OutputContinuation != 0 || calls[0].ParentCallEvidenceID != 0 ||
		!calls[0].OutputLimitReached || calls[0].Status != queue.LLMCallFailed ||
		calls[0].Outcome.Status != queue.LLMCallProviderFailed ||
		calls[0].Candidate != client.fixtures[0].candidate {
		t.Fatalf("terminal output-limit evidence=%#v", calls)
	}
}

func TestFreshSchemaSourceBodyCorrectionContinuesSamePersistedContextWithoutBlockingAnotherJob(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for source-body continuation evidence coverage")
	}
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise persisted source-body continuation", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "source-body-continuation-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "const total = left - right;\nreturn total;"},
		{candidate: "A"},
		{candidate: "left + right"},
	}}
	service := &Service{
		repo: repository, stationClient: client, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	base := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service, ctx: ctx, claim: claim,
	}, "source-body-integration")
	runtime := base
	runtime.MaxAttempts = assemblyline.MaxSourceBodyAttempts
	correct := base.Correct
	runtime.Correct = func(
		job assemblyline.PortableJob,
		modelName string,
		correction assemblyline.SourceBodyCorrection,
	) (assemblyline.PortableResult, error) {
		classification, err := assemblyline.NewApplicationClassificationJob(
			assemblyline.ApplicationClassificationInput{
				UserRequest: "classify an unrelated interface",
			},
		)
		if err != nil {
			return assemblyline.PortableResult{}, err
		}
		if _, err := runDirectCodingSemanticLeafCall(
			base, modelName, "unrelated-classification", classification, nil,
			func(candidate string) (string, error) {
				decoded, err := assemblyline.DecodeApplicationClassification(
					assemblyline.ApplicationClassificationInput{
						UserRequest: "classify an unrelated interface",
					},
					candidate,
				)
				if err != nil {
					return "", err
				}
				return string(decoded.Surface), nil
			},
		); err != nil {
			return assemblyline.PortableResult{}, err
		}
		// Configuration may change while this persisted job is alive. The
		// correction must retain the parent's frozen native model context.
		service.inferenceContextTokens = "16384"
		return correct(job, modelName, correction)
	}
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)",
		Behavior:  "Return the sum of left and right.",
	}
	source, err := runDirectCodingLanguageFragmentWorker(
		runtime,
		"fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "sum-body", Input: input,
			Validate: func(
				input assemblyline.FragmentGenerationInput,
				body string,
			) (string, error) {
				const wrong = "left - right"
				if start := strings.Index(body, wrong); start >= 0 {
					defect, defectErr := assemblyline.NewSourceBodyDefect(
						body,
						start,
						start+len(wrong),
						"Which expression computes the required sum?",
						fmt.Errorf("subtraction does not satisfy the required sum"),
					)
					if defectErr != nil {
						return "", defectErr
					}
					return "", defect
				}
				return validateDirectCodingJavaScriptFragment(input, body)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(source, input.Signature) != 1 ||
		!strings.Contains(source, "const total = left + right;") ||
		!strings.Contains(source, "return total;") {
		t.Fatalf("code-assembled source=%q", source)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[0].Outcome == nil || calls[1].Outcome == nil ||
		calls[2].Outcome == nil ||
		calls[0].WorkKind != string(assemblyline.WorkFragmentGeneration) ||
		calls[0].Iteration != 1 || calls[0].Outcome.Status != queue.LLMCallRejected ||
		calls[1].WorkKind != string(assemblyline.WorkApplicationClassify) ||
		calls[1].Iteration != 1 || calls[1].Outcome.Status != queue.LLMCallAccepted ||
		calls[2].WorkID != calls[0].WorkID || calls[2].Model != calls[0].Model ||
		calls[2].Iteration != 2 || calls[2].OutputContinuation != 0 ||
		calls[2].ParentCallEvidenceID != calls[0].ID ||
		calls[2].OutputLimitReached || calls[2].Outcome.Status != queue.LLMCallAccepted ||
		calls[0].ContextTokens != 8192 || calls[2].ContextTokens != calls[0].ContextTokens ||
		calls[2].SourceBaseCandidate != "const total = left - right;\nreturn total;" ||
		calls[2].SourceStartByte != len("const total = ") ||
		calls[2].SourceEndByte != len("const total = left - right") {
		t.Fatalf("source-body continuation evidence=%#v", calls)
	}
	const wantedCorrectionInput = "Which expression computes the required sum?\n\nleft - right"
	if len(client.prepared) != 3 || client.prepared[2].Prompt != wantedCorrectionInput ||
		client.prepared[0].MaxOutputTokens != -1 ||
		client.prepared[2].MaxOutputTokens != -1 ||
		strings.Contains(client.prepared[2].Prompt, "const total") ||
		strings.Contains(client.prepared[2].Prompt, "return total") ||
		strings.Contains(client.prepared[2].Prompt, input.Signature) {
		t.Fatalf("correction model input was not the exact mutable span: %#v", client.prepared)
	}
}

func TestFreshSchemaExactStationPipelineJournalsDecoderAndProviderOutcomes(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for exact station pipeline evidence coverage")
	}
	pool, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise station evidence pipeline", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "station-evidence-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	lateEntered := make(chan struct{})
	lateRelease := make(chan struct{})
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "A"},
		{candidate: "unrecognized artifact"},
		{partial: []byte(`{"created_at":"2026-08-31T12:00:00Z","response":"partial`)},
		{candidate: "B"},
		{candidate: "C", entered: lateEntered, release: lateRelease},
	}}
	service := &Service{
		repo: repository, stationClient: client, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service, ctx: ctx, claim: claim,
	}, "evidence-integration")

	classification, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	value, err := runDirectCodingSemanticLeafCall(
		runtime, "fixture-model", "classification", classification, nil,
		func(candidate string) (string, error) {
			decoded, err := assemblyline.DecodeApplicationClassification(
				assemblyline.ApplicationClassificationInput{
					UserRequest: "classify one interface",
				},
				candidate,
			)
			if err != nil {
				return "", err
			}
			return string(decoded.Surface), nil
		},
	)
	if err != nil || value != string(assemblyline.ApplicationSurfaceBrowser) {
		t.Fatalf("classification=%q err=%v", value, err)
	}

	artifact, err := assemblyline.NewArtifactHandlingJob(assemblyline.ArtifactHandlingInput{
		UserRequest: "preserve ARTIFACT_1", Token: "ARTIFACT_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifactResult, err := runtime.Execute(artifact, "fixture-model")
	if err != nil {
		t.Fatalf("dispatch artifact station: %v", err)
	}
	decoderErr := fmt.Errorf("artifact decoder rejected the exact candidate")
	if err := runtime.Finalize(artifact, artifactResult, decoderErr); err != nil {
		t.Fatalf("persist artifact decoder rejection: %v", err)
	}

	partial, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify another interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Execute(partial, "fixture-model"); err == nil {
		t.Fatal("partial provider response was accepted")
	}
	if _, err := runtime.Execute(partial, "fixture-model"); err == nil {
		t.Fatal("duplicate exact work was redispatched")
	}
	if client.calls != 3 {
		t.Fatalf("provider invocations=%d want 3", client.calls)
	}
	inflight, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify an inflight interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	inflightResult, err := runtime.Execute(inflight, "fixture-model")
	if err != nil {
		t.Fatalf("dispatch inflight station: %v", err)
	}
	late, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify a late interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	lateResult := make(chan error, 1)
	go func() {
		_, executeErr := runtime.Execute(late, "fixture-model")
		lateResult <- executeErr
	}()
	<-lateEntered
	if _, err := pool.Exec(ctx, `
		UPDATE job_step_attempts SET status='canceled',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID); err != nil {
		t.Fatal(err)
	}
	if err := runtime.Finalize(inflight, inflightResult, nil); !errors.Is(
		err, queue.ErrLLMCallTerminalizedByAttempt,
	) {
		t.Fatalf("finalize canceled inflight call err=%v", err)
	}
	close(lateRelease)
	if err := <-lateResult; !errors.Is(
		err, queue.ErrLLMCallTerminalizedByAttempt,
	) {
		t.Fatalf("late terminal station err=%v", err)
	}
	if client.calls != 5 {
		t.Fatalf("provider invocations=%d want 5", client.calls)
	}

	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 5 || calls[0].Outcome == nil || calls[1].Outcome == nil ||
		calls[2].Outcome == nil || calls[3].Outcome != nil || calls[4].Outcome == nil ||
		calls[0].Outcome.Status != queue.LLMCallAccepted ||
		calls[1].Outcome.Status != queue.LLMCallRejected ||
		calls[2].Outcome.Status != queue.LLMCallProviderFailed ||
		calls[4].Outcome.Status != queue.LLMCallInterrupted ||
		!calls[2].RawResponsePresent || string(calls[2].RawResponse) != string(client.fixtures[2].partial) ||
		!calls[3].ProviderReceiptPresent || !calls[3].RawResponsePresent ||
		calls[4].ProviderReceiptPresent || calls[4].RawResponsePresent {
		t.Fatalf("station call evidence=%#v", calls)
	}
	if calls[0].WorkKind == calls[1].WorkKind {
		t.Fatalf("fixtures did not cross unrelated station kinds: %#v", calls)
	}
	_ = pool
}

func listAllWorkerLLMCallEvidence(
	ctx context.Context,
	repository *queue.Repository,
	jobID int64,
) ([]queue.LLMCallEvidence, error) {
	items := make([]queue.LLMCallEvidence, 0)
	afterID := int64(0)
	for {
		page, err := repository.ListLLMCallEvidenceForJob(
			ctx, jobID, afterID, queue.MaxLLMCallEvidencePageSize,
		)
		if err != nil {
			return nil, err
		}
		if len(page) == 0 {
			return items, nil
		}
		items = append(items, page...)
		afterID = page[len(page)-1].ID
	}
}

func exactEvidenceSuccessfulGeneration(
	prepared llm.PreparedModel,
	candidate string,
) (llm.PreparedGeneration, error) {
	return exactEvidenceCompletedGeneration(prepared, candidate, "stop", 11, 7)
}

func exactEvidenceLengthGeneration(
	prepared llm.PreparedModel,
	candidate string,
	promptTokens int,
	outputTokens int,
) (llm.PreparedGeneration, error) {
	if promptTokens == 0 {
		promptTokens = 11
	}
	if outputTokens <= 0 {
		outputTokens = prepared.MaxOutputTokens
		if outputTokens == -1 {
			outputTokens = 7
		}
	}
	return exactEvidenceCompletedGeneration(
		prepared, candidate, "length", promptTokens, outputTokens,
	)
}

func exactEvidenceCompletedGeneration(
	prepared llm.PreparedModel,
	candidate string,
	doneReason string,
	promptTokens int,
	outputTokens int,
) (llm.PreparedGeneration, error) {
	raw := []byte(fmt.Sprintf(
		`{"created_at":"2026-08-31T12:00:00Z","response":%q,"done":true,"done_reason":%q,"total_duration":19,"load_duration":2,"prompt_eval_count":%d,"prompt_eval_duration":3,"eval_count":%d,"eval_duration":5}`,
		candidate, doneReason, promptTokens, outputTokens,
	))
	decoded, err := llm.DecodeExactPreparedResponseForProtocol(prepared.Protocol, 200, raw)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	requestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	responseDigest := sha256.Sum256(raw)
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestDispatched,
		Content:                    candidate, ProviderRequestSHA256: requestSHA, ProviderHTTPStatus: 200,
		ProviderResponseDisposition: decoded.Disposition,
		ProviderResponseComplete:    true, ProviderResponseBytesKnown: true,
		ProviderContentEncoding:       llm.NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseSHA256:        hex.EncodeToString(responseDigest[:]),
		ProviderResponseBytes:         int64(len(raw)),
		ProviderResponseCaptureSHA256: hex.EncodeToString(responseDigest[:]),
		ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: raw,
		ProviderDonePresent: decoded.DonePresent, ProviderDone: decoded.Done,
		ProviderDoneReason: decoded.DoneReason,
		UsagePresent:       decoded.UsagePresent, Usage: decoded.Usage,
	}, nil
}

func exactEvidencePartialGeneration(
	prepared llm.PreparedModel,
	raw []byte,
) llm.PreparedGeneration {
	requestSHA, _ := llm.ExactPreparedRequestSHA256(prepared)
	digest := sha256.Sum256(raw)
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestDispatched,
		ProviderRequestSHA256:      requestSHA, ProviderHTTPStatus: 200,
		ProviderResponseDisposition:   llm.ProviderResponseBodyReadError,
		ProviderContentEncoding:       llm.NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseCaptureSHA256: hex.EncodeToString(digest[:]),
		ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: append([]byte(nil), raw...),
	}
}

func freshWorkerEvidenceRepository(
	t *testing.T,
	databaseURL string,
) (*pgxpool.Pool, *queue.Repository) {
	t.Helper()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "omnidex_worker_evidence_test_" + hex.EncodeToString(nonce[:])
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatalf("install fresh worker evidence schema %q: %v", schema, err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(
			cleanupCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		); err != nil {
			t.Errorf("drop worker evidence schema %q: %v", schema, err)
		}
		pool.Close()
	})
	authority, err := modelconfig.Freeze(modelconfig.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return pool, queue.New(pool, authority, model.CodingScopeModeNormal)
}
