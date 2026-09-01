package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFreshSchemaReceiptlessInterruptedExactCallIsTerminalWithoutProviderCall(t *testing.T) {
	pool, repository, ctx, claimed := freshInterruptedRecoveryClaim(t, "terminal")
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	_, _, opening := reserveInterruptedInitialOpening(
		t, ctx, repository, claimed, job, "fixture-model", 8192,
	)
	claim2 := reclaimEvidenceAttemptForTest(t, ctx, pool, claimed, "terminal-recovery-2")
	client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: "A"}}}
	runtime := portableWorkerRuntime(&nativeRuntimeV3{
		svc: &Service{
			repo: repository, stationClient: client, inferenceContextTokens: "8192",
			runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
		},
		ctx: ctx, claim: claim2,
	}, "interrupted-terminal")

	if _, err := runtime.Execute(job, "fixture-model"); err == nil {
		t.Fatal("receipt-less interrupted call was not terminal")
	} else if !strings.Contains(err.Error(), "interrupted before one complete provider response") {
		t.Fatalf("interrupted recovery error=%v", err)
	}
	if client.calls != 0 || runtime.ProviderCalls == nil || runtime.ProviderCalls() != 0 {
		t.Fatalf("interrupted recovery invoked provider %d times", client.calls)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, claimed.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0].ID != opening.ID || calls[0].ProviderReceiptPresent ||
		calls[0].Outcome == nil || calls[0].Outcome.Status != queue.LLMCallInterrupted {
		t.Fatalf("interrupted evidence=%#v", calls)
	}
}

func TestLegacyContinuationAndReplacementEvidenceIsTerminalWithoutProviderCall(t *testing.T) {
	job, err := assemblyline.NewApplicationClassificationJob(
		assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"},
	)
	if err != nil {
		t.Fatal(err)
	}
	scope, err := portableModelScope(job.Kind)
	if err != nil {
		t.Fatal(err)
	}
	authority := model.StepAttemptAuthority{
		JobID: 19, Generation: 2, StepID: 7, Attempt: 3, WorkerID: "legacy-test-worker",
	}
	base := queue.LLMCallEvidence{
		JobID: authority.JobID, Generation: authority.Generation, StepID: authority.StepID,
		WorkID: job.ID, WorkKind: string(job.Kind), Scope: scope,
		RequestedModel: "fixture-model", Model: "fixture-model",
		Protocol:  string(llm.ExactPreparedProtocolPlainCompletionV4),
		Iteration: 1, DispatchAttempt: 1,
	}
	for name, mutate := range map[string]func(*queue.LLMCallEvidence){
		"output continuation": func(evidence *queue.LLMCallEvidence) {
			evidence.OutputContinuation = 1
			evidence.ParentCallEvidenceID = 11
		},
		"replacement dispatch": func(evidence *queue.LLMCallEvidence) {
			evidence.DispatchAttempt = 2
			evidence.ReplacesCallEvidenceID = 11
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			client := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: "A"}}}
			service := &Service{stationClient: client}
			evidence := base
			mutate(&evidence)
			recovery, err := service.recoverExactPortableStationEvidence(
				job, "fixture-model", authority, evidence,
			)
			if err == nil || recovery != nil {
				t.Fatalf("legacy state recovery=%#v err=%v", recovery, err)
			}
			if !strings.Contains(err.Error(), "unsupported continuation or replacement dispatch state") {
				t.Fatalf("legacy state error=%v", err)
			}
			if client.calls != 0 {
				t.Fatalf("legacy state invoked provider %d times", client.calls)
			}
		})
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
		ctx, "exercise terminal interrupted "+suffix+" dispatch", t.TempDir(),
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
		WorkID: job.ID, WorkKind: job.Kind, Iteration: 1,
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
