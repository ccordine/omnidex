package worker

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
)

func TestFreshSchemaRecoveryRecreatesOpaqueMapBeforeReplayingPersistedChild(
	t *testing.T,
) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for source-body recovery coverage")
	}
	pool, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(
		ctx, "exercise sequential opaque source recovery", t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim1, err := repository.ClaimNextStep(ctx, "opaque-recovery-worker-1")
	if err != nil || claim1 == nil || claim1.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim1, err)
	}
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Pick(left, right)",
		Behavior:  "Return one available input value.",
	}
	validator := opaqueRecoveryValidator(t)
	fragment, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	client1 := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "return missing;", nativeContext: "[70,81,92]"},
	}}
	service1 := &Service{
		repo: repository, stationClient: client1, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime1 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service1, ctx: ctx, claim: claim1,
	}, "opaque-source-recovery")
	initialResult, err := runtime1.Execute(fragment, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	initialBody, err := assemblyline.NormalizeSourceBodyResponse(initialResult.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	_, _, correction, validationErr := validateDirectCodingLanguageBody(
		runtime1.PathProvenance,
		directCodingLanguageGenerationJob{Subject: "pick-body", Input: input, Validate: validator},
		initialBody,
	)
	if validationErr == nil || correction == nil {
		t.Fatalf("initial opaque correction=%#v validation=%v", correction, validationErr)
	}
	if err := runtime1.Finalize(fragment, initialResult, validationErr); err != nil {
		t.Fatal(err)
	}

	claim2 := reclaimEvidenceAttemptForTest(
		t, ctx, pool, claim1, "opaque-recovery-worker-2",
	)
	client2 := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "A"},
	}}
	service2 := &Service{
		repo: repository, stationClient: client2, inferenceContextTokens: "16384",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime2 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service2, ctx: ctx, claim: claim2,
	}, "opaque-source-recovery")
	runtime2.MaxAttempts = assemblyline.MaxSourceBodyAttempts
	firstRecovered, err := runDirectCodingLanguageFragmentWorker(
		runtime2, "fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "pick-body", Input: input, Validate: validator,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if client2.calls != 1 || !strings.Contains(firstRecovered, "left") {
		t.Fatalf("first recovery calls=%d source=%q", client2.calls, firstRecovered)
	}

	claim3 := reclaimEvidenceAttemptForTest(
		t, ctx, pool, claim2, "opaque-recovery-worker-3",
	)
	client3 := &exactEvidenceStationClient{}
	service3 := &Service{
		repo: repository, stationClient: client3, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime3 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service3, ctx: ctx, claim: claim3,
	}, "opaque-source-recovery")
	runtime3.MaxAttempts = assemblyline.MaxSourceBodyAttempts
	secondRecovered, err := runDirectCodingLanguageFragmentWorker(
		runtime3, "fixture-model",
		directCodingLanguageGenerationJob{
			Subject: "pick-body", Input: input, Validate: validator,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if client3.calls != 0 || secondRecovered != firstRecovered {
		t.Fatalf(
			"sequential child replay calls=%d source=%q want=%q",
			client3.calls, secondRecovered, firstRecovered,
		)
	}
	if runtime3.ProviderCalls == nil || runtime3.ProviderCalls() != 0 {
		t.Fatalf("sequential child replay reported a provider call")
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].Iteration != 1 || calls[1].Iteration != 2 ||
		calls[1].OutputContinuation != 0 ||
		calls[1].ParentCallEvidenceID != calls[0].ID ||
		calls[1].OutputLimitReached || calls[1].Outcome == nil ||
		calls[1].Outcome.Status != queue.LLMCallAccepted ||
		calls[1].ContextTokens != calls[0].ContextTokens ||
		calls[1].Candidate != "A" || calls[1].MaxOutputTokens != 8 {
		t.Fatalf("opaque recovery evidence=%#v", calls)
	}
	assertExactEvidenceRequestContext(t, calls[1].ProviderRequest, "[70,81,92]")
}

func opaqueRecoveryValidator(t *testing.T) directCodingLanguageFragmentValidator {
	t.Helper()
	left, err := assemblyline.NewOpaqueModelChoice("Use the first available input", "left")
	if err != nil {
		t.Fatal(err)
	}
	right, err := assemblyline.NewOpaqueModelChoice("Use the second available input", "right")
	if err != nil {
		t.Fatal(err)
	}
	return func(
		input assemblyline.FragmentGenerationInput,
		body string,
	) (string, error) {
		if body == "return missing;" {
			defect, err := assemblyline.NewSourceBodyIdentifierDefect(
				body,
				len("return "),
				len("return missing"),
				"Which available input should replace this unresolved reference?",
				fmt.Errorf("unresolved identifier missing"),
				[]assemblyline.OpaqueModelChoice{left, right},
			)
			if err != nil {
				return "", err
			}
			return "", defect
		}
		return validateDirectCodingJavaScriptFragment(input, body)
	}
}
