package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestFreshSchemaExpiredAttemptReplaysAcceptedLeavesAndContinuesExactSourceChild(
	t *testing.T,
) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for source-body recovery coverage")
	}
	pool, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(
		ctx, "exercise source continuation after an expired attempt", t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim1, err := repository.ClaimNextStep(ctx, "source-recovery-worker-1")
	if err != nil || claim1 == nil || claim1.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim1, err)
	}
	client1 := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "A"},
		{candidate: "const total = left - right;\nreturn total;", nativeContext: "[41,52,63]"},
	}}
	service1 := &Service{
		repo: repository, stationClient: client1, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime1 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service1, ctx: ctx, claim: claim1,
	}, "source-recovery")

	classification, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	classificationResult, err := runtime1.Execute(classification, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime1.Finalize(classification, classificationResult, nil); err != nil {
		t.Fatal(err)
	}

	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)",
		Behavior:  "Return the sum of left and right.",
	}
	fragment, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	initialResult, err := runtime1.Execute(fragment, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	initialBody, err := assemblyline.NormalizeSourceBodyResponse(initialResult.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	initialDefect := sourceRecoverySumDefect(t, initialBody)
	if err := runtime1.Finalize(fragment, initialResult, initialDefect); err != nil {
		t.Fatal(err)
	}
	if client1.calls != 2 {
		t.Fatalf("initial provider calls=%d want 2", client1.calls)
	}

	claim2 := reclaimEvidenceAttemptForTest(
		t, ctx, pool, claim1, "source-recovery-worker-2",
	)
	client2 := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "left + right"},
	}}
	service2 := &Service{
		repo: repository, stationClient: client2, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime2 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service2, ctx: ctx, claim: claim2,
	}, "source-recovery")
	runtime2.MaxAttempts = assemblyline.MaxSourceBodyAttempts

	replayedClassification, err := runtime2.Execute(classification, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime2.Finalize(classification, replayedClassification, nil); err != nil {
		t.Fatal(err)
	}
	if client2.calls != 0 {
		t.Fatalf("accepted leaf replay invoked provider %d times", client2.calls)
	}
	if runtime2.ProviderCalls == nil || runtime2.ProviderCalls() != 0 {
		t.Fatalf("accepted leaf replay reported a provider call")
	}

	source, err := runDirectCodingLanguageFragmentWorker(
		runtime2,
		"fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "sum-body", Input: input,
			Validate: func(
				input assemblyline.FragmentGenerationInput,
				body string,
			) (string, error) {
				if strings.Contains(body, "left - right") {
					return "", sourceRecoverySumDefect(t, body)
				}
				return validateDirectCodingJavaScriptFragment(input, body)
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if client2.calls != 1 || len(client2.prepared) != 1 {
		t.Fatalf("recovered provider calls=%d prepared=%d want one correction", client2.calls, len(client2.prepared))
	}
	if runtime2.ProviderCalls() != 1 {
		t.Fatalf("recovered runtime provider calls=%d want 1", runtime2.ProviderCalls())
	}
	const correctionInput = "Which expression computes the required sum?\n\nleft - right"
	if client2.prepared[0].Prompt != correctionInput {
		t.Fatalf("recovered correction prompt=%q", client2.prepared[0].Prompt)
	}
	if !strings.Contains(source, "const total = left + right;") ||
		!strings.Contains(source, "return total;") {
		t.Fatalf("recovered source=%q", source)
	}

	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || calls[0].Outcome == nil || calls[1].Outcome == nil ||
		calls[2].Outcome == nil || calls[0].Outcome.Status != queue.LLMCallAccepted ||
		calls[1].Outcome.Status != queue.LLMCallRejected ||
		calls[2].Outcome.Status != queue.LLMCallAccepted ||
		calls[2].ParentCallEvidenceID != calls[1].ID ||
		calls[2].StepAttempt != claim2.Authority.Attempt ||
		calls[2].WorkerID != claim2.Authority.WorkerID ||
		calls[2].Model != calls[1].Model || calls[2].WorkInput != nil || len(calls[1].WorkInput) == 0 {
		t.Fatalf("recovered evidence=%#v", calls)
	}
	assertExactEvidenceRequestContext(t, calls[2].ProviderRequest, "[41,52,63]")
}
