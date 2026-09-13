package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

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
