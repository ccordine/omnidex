package worker

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFreshSchemaInterruptedInitialOpeningGetsOneIdenticalPhysicalReplacement(t *testing.T) {
	pool, repository, ctx, claimed := freshInterruptedRecoveryClaim(t, "initial")
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	call, prepared, opening := reserveInterruptedInitialOpening(
		t, ctx, repository, claimed, job, "fixture-model", 8192,
	)
	claim2 := reclaimEvidenceAttemptForTest(t, ctx, pool, claimed, "initial-recovery-2")
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: "A"}}}
	runtime := portableWorkerRuntime(&nativeRuntimeV3{
		svc: &Service{
			repo: repository, stationClient: client, inferenceContextTokens: "16384",
			runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
		},
		ctx: ctx, claim: claim2,
	}, "interrupted-initial")
	value, err := runDirectCodingSemanticLeafCall(
		runtime, "fixture-model", "classification", job, nil,
		func(candidate string) (string, error) {
			decoded, err := assemblyline.DecodeApplicationClassification(
				assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
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
	if client.calls != 1 || runtime.ProviderCalls == nil || runtime.ProviderCalls() != 1 {
		t.Fatalf("replacement provider calls=%d", client.calls)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, claimed.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].ID != opening.ID || calls[0].Outcome == nil ||
		calls[0].Outcome.Status != queue.LLMCallInterrupted || calls[0].ProviderReceiptPresent ||
		calls[1].Outcome == nil || calls[1].Outcome.Status != queue.LLMCallAccepted {
		t.Fatalf("initial replacement evidence=%#v", calls)
	}
	assertExactPhysicalReplacement(t, calls[0], calls[1])
	if calls[1].ContextTokens != prepared.ContextTokens ||
		calls[1].MaxOutputTokens != prepared.MaxOutputTokens {
		t.Fatalf("replacement changed frozen context authority: %#v", calls)
	}
	call.DispatchAttempt = 2
	call.ReplacesCallID = calls[1].ID
	if _, err := (&Service{repo: repository}).reserveExactStationCallEvidence(
		ctx, claim2.Authority, call, prepared,
	); err == nil {
		t.Fatal("completed replacement receipt was physically redispatched")
	}
}

func TestFreshSchemaInterruptedCorrectionOpeningGetsOneIdenticalPhysicalReplacement(t *testing.T) {
	pool, repository, ctx, claimed := freshInterruptedRecoveryClaim(t, "correction")
	input := assemblyline.FragmentGenerationInput{
		Language: "javascript", Dialect: "ECMAScript 2022",
		Signature: "function Sum(left, right)", Behavior: "Return the sum of left and right.",
	}
	fragment, err := assemblyline.NewFragmentGenerationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	client1 := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{
		candidate: "const total = left - right;\nreturn total;",
	}}}
	service1 := &Service{
		repo: repository, stationClient: client1, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime1 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service1, ctx: ctx, claim: claimed,
	}, "interrupted-correction")
	initialResult, err := runtime1.Execute(fragment, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	body, err := assemblyline.ExtractFragmentGenerationSourceBody(fragment, initialResult.Candidate)
	if err != nil {
		t.Fatal(err)
	}
	defect := sourceRecoverySumDefect(t, body)
	if err := runtime1.Finalize(fragment, initialResult, defect); err != nil {
		t.Fatal(err)
	}
	initialCalls, err := listAllWorkerLLMCallEvidence(ctx, repository, claimed.Job.ID)
	if err != nil || len(initialCalls) != 1 {
		t.Fatalf("initial calls=%#v err=%v", initialCalls, err)
	}
	rejectedCall, rejectedPrepared, err := recreatePersistedExactStationCall(
		fragment, "fixture-model", initialCalls[0], nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	rejectedCall.DispatchAttempt = 2
	rejectedCall.ReplacesCallID = initialCalls[0].ID
	if _, err := service1.reserveExactStationCallEvidence(
		ctx, claimed.Authority, rejectedCall, rejectedPrepared,
	); err == nil {
		t.Fatal("semantically rejected completed receipt was physically redispatched")
	}
	correction, err := defect.Correction(body)
	if err != nil {
		t.Fatal(err)
	}
	correctionPrompt, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	correctionEvidence, err := correction.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	maxOutput, err := queue.ExpectedSourceBodyCorrectionMaxOutputTokens(0, false, 8192)
	if err != nil {
		t.Fatal(err)
	}
	correctionCall := exactStationCall{
		WorkID: fragment.ID, WorkKind: fragment.Kind, Iteration: 2,
		DispatchAttempt: 1, ParentCallID: initialCalls[0].ID,
		Prompt: correctionPrompt, ContextTokens: 8192, MaxOutputTokens: maxOutput,
		SourceCorrection: &correctionEvidence,
	}
	correctionPrepared, err := prepareExactStationCall(correctionCall, "fixture-model", nil)
	if err != nil {
		t.Fatal(err)
	}
	opening, err := service1.reserveExactStationCallEvidence(
		ctx, claimed.Authority, correctionCall, correctionPrepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim2 := reclaimEvidenceAttemptForTest(t, ctx, pool, claimed, "correction-recovery-2")
	client2 := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: "left + right"}}}
	runtime2 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: &Service{
			repo: repository, stationClient: client2, inferenceContextTokens: "16384",
			runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
		},
		ctx: ctx, claim: claim2,
	}, "interrupted-correction")
	runtime2.MaxAttempts = assemblyline.MaxSourceBodyAttempts
	source, err := runDirectCodingLanguageFragmentWorker(
		runtime2, "fixture-model", directCodingLanguageGenerationJob{
			Subject: "sum-body", Input: input,
			Validate: func(input assemblyline.FragmentGenerationInput, candidate string) (string, error) {
				if strings.Contains(candidate, "left - right") {
					return "", sourceRecoverySumDefect(t, candidate)
				}
				return validateDirectCodingJavaScriptFragment(input, candidate)
			},
		},
	)
	if err != nil || !strings.Contains(source, "left + right") {
		t.Fatalf("recovered source=%q err=%v", source, err)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, claimed.Job.ID)
	if err != nil || len(calls) != 3 {
		t.Fatalf("correction calls=%#v err=%v", calls, err)
	}
	if calls[1].ID != opening.ID || calls[1].Outcome == nil ||
		calls[1].Outcome.Status != queue.LLMCallInterrupted || calls[2].Outcome == nil ||
		calls[2].Outcome.Status != queue.LLMCallAccepted {
		t.Fatalf("correction replacement evidence=%#v", calls)
	}
	assertExactPhysicalReplacement(t, calls[1], calls[2])
	if calls[2].ContextTokens != calls[0].ContextTokens ||
		calls[2].ModelInput != correctionPrompt ||
		strings.Contains(calls[2].ModelInput, input.Signature) ||
		strings.Contains(calls[2].ModelInput, "const total") {
		t.Fatalf("correction replacement widened or changed model context: %#v", calls[2])
	}
}

func TestFreshSchemaInterruptedOutputContinuationGetsOneIdenticalPhysicalReplacement(t *testing.T) {
	pool, repository, ctx, claimed := freshInterruptedRecoveryClaim(t, "output")
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	call, prepared, opening := reserveInterruptedInitialOpening(
		t, ctx, repository, claimed, job, "fixture-model", 8192,
	)
	generation, err := exactEvidenceLengthGeneration(
		prepared, "unfinished output", 11, prepared.MaxOutputTokens,
	)
	if err != nil {
		t.Fatal(err)
	}
	limitErr := llm.ValidateExactPreparedGenerationForRequest(prepared, generation)
	parent, err := (&Service{repo: repository}).finalizeExactStationCallEvidence(
		ctx, claimed.Authority, opening.ID, prepared, generation, limitErr, time.Millisecond,
	)
	if err != nil {
		t.Fatal(err)
	}
	failedCall := call
	failedCall.DispatchAttempt = 2
	failedCall.ReplacesCallID = parent.ID
	if _, err := (&Service{repo: repository}).reserveExactStationCallEvidence(
		ctx, claimed.Authority, failedCall, prepared,
	); err == nil {
		t.Fatal("provider-failed completed receipt was physically redispatched")
	}
	call.ParentCallID = parent.ID
	call.OutputContinuation = 1
	call.DispatchAttempt = 1
	call.MaxOutputTokens = call.ContextTokens - generation.Usage.PromptEvalCount
	continuedPrepared, err := prepareExactStationCall(call, "fixture-model", nil)
	if err != nil {
		t.Fatal(err)
	}
	continuedOpening, err := (&Service{repo: repository}).reserveExactStationCallEvidence(
		ctx, claimed.Authority, call, continuedPrepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim2 := reclaimEvidenceAttemptForTest(t, ctx, pool, claimed, "output-recovery-2")
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: "A"}}}
	runtime := portableWorkerRuntime(&nativeRuntimeV3{
		svc: &Service{
			repo: repository, stationClient: client, inferenceContextTokens: "16384",
			runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
		},
		ctx: ctx, claim: claim2,
	}, "interrupted-output")
	if _, err := runDirectCodingSemanticLeafCall(
		runtime, "fixture-model", "classification", job, nil,
		func(candidate string) (string, error) { return candidate, nil },
	); err != nil {
		t.Fatal(err)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, claimed.Job.ID)
	if err != nil || len(calls) != 3 {
		t.Fatalf("output calls=%#v err=%v", calls, err)
	}
	if calls[1].ID != continuedOpening.ID || calls[1].Outcome == nil ||
		calls[1].Outcome.Status != queue.LLMCallInterrupted || calls[2].Outcome == nil ||
		calls[2].Outcome.Status != queue.LLMCallAccepted ||
		calls[2].OutputContinuation != 1 {
		t.Fatalf("output replacement evidence=%#v", calls)
	}
	assertExactPhysicalReplacement(t, calls[1], calls[2])
}

func TestFreshSchemaSecondInterruptedPhysicalDispatchCannotBeReplaced(t *testing.T) {
	pool, repository, ctx, claimed := freshInterruptedRecoveryClaim(t, "second")
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	call, prepared, opening := reserveInterruptedInitialOpening(
		t, ctx, repository, claimed, job, "fixture-model", 8192,
	)
	claim2 := reclaimEvidenceAttemptForTest(t, ctx, pool, claimed, "second-recovery-2")
	call.DispatchAttempt = 2
	call.ReplacesCallID = opening.ID
	replacement, err := (&Service{repo: repository}).reserveExactStationCallEvidence(
		ctx, claim2.Authority, call, prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim3 := reclaimEvidenceAttemptForTest(t, ctx, pool, claim2, "second-recovery-3")
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: "A"}}}
	runtime := portableWorkerRuntime(&nativeRuntimeV3{
		svc: &Service{
			repo: repository, stationClient: client, inferenceContextTokens: "8192",
			runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
		},
		ctx: ctx, claim: claim3,
	}, "second-interruption")
	if _, err := runtime.Execute(job, "fixture-model"); err == nil {
		t.Fatal("second interrupted physical dispatch was replaced")
	}
	if client.calls != 0 || runtime.ProviderCalls == nil || runtime.ProviderCalls() != 0 {
		t.Fatalf("second interruption invoked provider %d times", client.calls)
	}
	call.DispatchAttempt = 1
	call.ReplacesCallID = 0
	if _, err := (&Service{repo: repository}).reserveExactStationCallEvidence(
		ctx, claim3.Authority, call, prepared,
	); err == nil {
		t.Fatal("database accepted another first dispatch across reclaimed attempts")
	}
	call.DispatchAttempt = 2
	call.ReplacesCallID = replacement.ID
	if _, err := (&Service{repo: repository}).reserveExactStationCallEvidence(
		ctx, claim3.Authority, call, prepared,
	); err == nil {
		t.Fatal("database accepted a third physical dispatch")
	}
}

func freshInterruptedRecoveryClaim(
	t *testing.T,
	suffix string,
) (*pgxpool.Pool, *queue.Repository, context.Context, *model.ClaimedStep) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for interrupted dispatch recovery coverage")
	}
	pool, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(
		ctx, "exercise interrupted "+suffix+" dispatch", t.TempDir(),
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, suffix+"-recovery-1")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	return pool, repository, ctx, claim
}

func reserveInterruptedInitialOpening(
	t *testing.T,
	ctx context.Context,
	repository *queue.Repository,
	claim *model.ClaimedStep,
	job assemblyline.PortableJob,
	modelName string,
	contextTokens int,
) (exactStationCall, llm.PreparedModel, queue.LLMCallEvidence) {
	t.Helper()
	prompt, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	maximum, err := queue.ExpectedPortableStationMaxOutputTokens(job, contextTokens)
	if err != nil {
		t.Fatal(err)
	}
	call := exactStationCall{
		WorkID: job.ID, WorkKind: job.Kind, Iteration: 1, DispatchAttempt: 1,
		Prompt: prompt, ContextTokens: contextTokens, MaxOutputTokens: maximum,
	}
	prepared, err := prepareExactStationCall(call, modelName, nil)
	if err != nil {
		t.Fatal(err)
	}
	opening, err := (&Service{repo: repository}).reserveExactStationCallEvidence(
		ctx, claim.Authority, call, prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	return call, prepared, opening
}

func assertExactPhysicalReplacement(
	t *testing.T,
	interrupted queue.LLMCallEvidence,
	replacement queue.LLMCallEvidence,
) {
	t.Helper()
	if interrupted.DispatchAttempt != 1 || replacement.DispatchAttempt != 2 ||
		replacement.ReplacesCallEvidenceID != interrupted.ID ||
		replacement.WorkID != interrupted.WorkID || replacement.WorkKind != interrupted.WorkKind ||
		replacement.Iteration != interrupted.Iteration ||
		replacement.OutputContinuation != interrupted.OutputContinuation ||
		replacement.ParentCallEvidenceID != interrupted.ParentCallEvidenceID ||
		replacement.Model != interrupted.Model || replacement.RequestedModel != interrupted.RequestedModel ||
		replacement.Protocol != interrupted.Protocol ||
		replacement.SystemEnvelope != interrupted.SystemEnvelope ||
		replacement.ModelInput != interrupted.ModelInput ||
		!bytes.Equal(replacement.ProviderRequest, interrupted.ProviderRequest) ||
		replacement.ContextTokens != interrupted.ContextTokens ||
		replacement.MaxOutputTokens != interrupted.MaxOutputTokens ||
		replacement.OutputLimitMode != interrupted.OutputLimitMode ||
		replacement.SourceBaseCandidate != interrupted.SourceBaseCandidate ||
		replacement.SourceBaseSHA256 != interrupted.SourceBaseSHA256 ||
		replacement.SourceStartByte != interrupted.SourceStartByte ||
		replacement.SourceEndByte != interrupted.SourceEndByte ||
		replacement.SourceQuestion != interrupted.SourceQuestion ||
		replacement.SourceQuestionSHA256 != interrupted.SourceQuestionSHA256 {
		t.Fatalf("physical replacement changed semantic call authority: interrupted=%#v replacement=%#v", interrupted, replacement)
	}
}
