package worker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

func TestFreshSchemaOutputLimitIsOneTerminalProviderCall(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for output-limit evidence coverage")
	}
	_, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(
		ctx, "exercise terminal output limit", t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "hard-output-bound-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "incomplete output", doneReason: "length", promptTokens: 11},
	}}
	service := &Service{
		repo: repository, stationClient: client, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service, ctx: ctx, claim: claim,
	}, "hard-output-bound")
	classification, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Execute(classification, "fixture-model"); err == nil {
		t.Fatal("output-limit result was accepted")
	} else {
		var limit *llm.ExactPreparedOutputLimitReachedError
		if !errors.As(err, &limit) {
			t.Fatalf("hard output error=%v", err)
		}
	}
	if client.calls != 1 || runtime.ProviderCalls == nil || runtime.ProviderCalls() != 1 {
		t.Fatalf("provider calls=%d", client.calls)
	}
	if _, err := runtime.Execute(classification, "fixture-model"); err == nil {
		t.Fatal("persisted output-limit failure was accepted")
	}
	if client.calls != 1 || runtime.ProviderCalls() != 1 {
		t.Fatalf("persisted output-limit failure was redispatched: provider=%d", client.calls)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].Outcome == nil ||
		calls[0].Iteration != 1 || calls[0].OutputContinuation != 0 ||
		calls[0].ParentCallEvidenceID != 0 || !calls[0].OutputLimitReached ||
		calls[0].Outcome.Status != queue.LLMCallProviderFailed {
		t.Fatalf("terminal output evidence=%#v", calls)
	}
}
