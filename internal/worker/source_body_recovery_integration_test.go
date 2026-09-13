package worker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestFreshSchemaExpiredAttemptReplaysTerminalOutputLimitWithoutDispatch(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for output-limit recovery coverage")
	}
	pool, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(
		ctx, "exercise restart-durable terminal output limit", t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim1, err := repository.ClaimNextStep(ctx, "output-recovery-worker-1")
	if err != nil || claim1 == nil || claim1.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim1, err)
	}
	classification, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := assemblyline.RenderPortableJob(classification)
	if err != nil {
		t.Fatal(err)
	}
	initialMaximum, err := queue.ExpectedPortableStationMaxOutputTokens(classification, 8192)
	if err != nil {
		t.Fatal(err)
	}
	call := exactStationCall{
		WorkInput: string(classification.Payload), WorkKind: classification.Kind, Iteration: 1,
		Prompt: prompt, ContextTokens: 8192, MaxOutputTokens: initialMaximum,
	}
	prepared, err := prepareExactStationCall(call, "fixture-model", nil)
	if err != nil {
		t.Fatal(err)
	}
	service1 := &Service{repo: repository}
	opening, err := service1.reserveExactStationCallEvidence(
		ctx, claim1.Authority, call, prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	const incompleteCandidate = "unfinished output that must not be decoded"
	generation, err := exactEvidenceLengthGeneration(
		prepared, incompleteCandidate, 11, initialMaximum,
	)
	if err != nil {
		t.Fatal(err)
	}
	limitErr := llm.ValidateExactPreparedGenerationForRequest(
		prepared, generation,
	)
	var limit *llm.ExactPreparedOutputLimitReachedError
	if !errors.As(limitErr, &limit) {
		t.Fatalf("length fixture error=%v", limitErr)
	}
	parent, err := service1.finalizeExactStationCallEvidence(
		ctx, claim1.Authority, opening.ID, prepared, generation, limitErr, time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}
	if parent.Outcome == nil || parent.Outcome.Status != queue.LLMCallProviderFailed {
		t.Fatalf("persisted incomplete parent=%#v", parent)
	}

	claim2 := reclaimEvidenceAttemptForTest(
		t, ctx, pool, claim1, "output-recovery-worker-2",
	)
	client2 := &exactEvidenceStationClient{}
	service2 := &Service{
		repo: repository, stationClient: client2, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime2 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service2, ctx: ctx, claim: claim2,
	}, "output-recovery")
	decodedCandidates := make([]string, 0, 1)
	_, err = runDirectCodingSemanticLeafCall(
		runtime2, "fixture-model", "classification", classification, nil,
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
		t.Fatal("persisted output-limit failure was accepted")
	}
	if !strings.Contains(err.Error(), limit.Error()) {
		t.Fatalf("persisted output-limit error=%v want=%v", err, limit)
	}
	if client2.calls != 0 || runtime2.ProviderCalls == nil || runtime2.ProviderCalls() != 0 {
		t.Fatalf("recovery provider calls=%d", client2.calls)
	}
	if len(decodedCandidates) != 0 {
		t.Fatalf("recovery semantic decoder candidates=%q", decodedCandidates)
	}
	if len(client2.prepared) != 0 {
		t.Fatalf("persisted output failure prepared a new request=%#v", client2.prepared)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].ID != parent.ID || calls[0].Outcome == nil ||
		calls[0].Outcome.Status != queue.LLMCallProviderFailed ||
		!calls[0].OutputLimitReached {
		t.Fatalf("terminal recovered output evidence=%#v", calls)
	}
}
