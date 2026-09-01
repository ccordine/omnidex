package worker

import (
	"context"
	"errors"
	"fmt"
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

func TestFreshSchemaExpiredAttemptResumesPersistedOutputLimitLineage(t *testing.T) {
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for output-limit recovery coverage")
	}
	pool, repository := freshWorkerEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(
		ctx, "exercise restart-durable output continuation", t.TempDir(),
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
		WorkID: classification.ID, WorkKind: classification.Kind, Iteration: 1,
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
	client2 := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{
		{candidate: "A"},
	}}
	service2 := &Service{
		repo: repository, stationClient: client2, inferenceContextTokens: "8192",
		runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
	}
	runtime2 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: service2, ctx: ctx, claim: claim2,
	}, "output-recovery")
	decodedCandidates := make([]string, 0, 1)
	value, err := runDirectCodingSemanticLeafCall(
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
	if err != nil || value != string(assemblyline.ApplicationSurfaceBrowser) {
		t.Fatalf("classification=%q err=%v", value, err)
	}
	if client2.calls != 1 || runtime2.ProviderCalls == nil || runtime2.ProviderCalls() != 1 {
		t.Fatalf("recovery provider calls=%d", client2.calls)
	}
	if len(decodedCandidates) != 1 || decodedCandidates[0] != "A" {
		t.Fatalf("recovery semantic decoder candidates=%q", decodedCandidates)
	}
	if len(client2.prepared) != 1 ||
		client2.prepared[0].MaxOutputTokens != 8192-limit.PromptTokens ||
		client2.prepared[0].Prompt != prompt ||
		client2.prepared[0].BaseModel != prepared.BaseModel {
		t.Fatalf("recovered continuation request=%#v", client2.prepared)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0].ID != parent.ID || calls[1].Outcome == nil ||
		calls[1].ParentCallEvidenceID != parent.ID ||
		calls[1].OutputContinuation != 1 || calls[1].Iteration != calls[0].Iteration ||
		calls[1].WorkID != calls[0].WorkID || calls[1].Model != calls[0].Model ||
		calls[1].StepAttempt != claim2.Authority.Attempt ||
		calls[1].Outcome.Status != queue.LLMCallAccepted {
		t.Fatalf("recovered output lineage=%#v", calls)
	}
}

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
		ctx, "exercise restart-durable source continuation", t.TempDir(),
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
		{candidate: "const total = left - right;\nreturn total;"},
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
		calls[2].Model != calls[1].Model || calls[2].WorkID != calls[1].WorkID {
		t.Fatalf("recovered evidence=%#v", calls)
	}
}

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
		{candidate: "return missing;"},
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
		repo: repository, stationClient: client2, inferenceContextTokens: "8192",
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
	if client2.calls != 1 || !strings.Contains(firstRecovered, "return left;") {
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
		calls[1].ParentCallEvidenceID != calls[0].ID ||
		calls[1].Candidate != "A" || calls[1].Outcome == nil ||
		calls[1].Outcome.Status != queue.LLMCallAccepted {
		t.Fatalf("opaque recovery evidence=%#v", calls)
	}
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
		const missing = "missing"
		if start := strings.Index(body, missing); start >= 0 {
			defect, err := assemblyline.NewSourceBodyIdentifierDefect(
				body,
				start,
				start+len(missing),
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

func sourceRecoverySumDefect(t *testing.T, body string) *assemblyline.SourceBodyDefect {
	t.Helper()
	const wrong = "left - right"
	start := strings.Index(body, wrong)
	if start < 0 {
		t.Fatalf("source body lacks expected defect: %q", body)
	}
	defect, err := assemblyline.NewSourceBodyDefect(
		body,
		start,
		start+len(wrong),
		"Which expression computes the required sum?",
		fmt.Errorf("subtraction does not satisfy the required sum"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return defect
}

func reclaimEvidenceAttemptForTest(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	claim *model.ClaimedStep,
	workerID string,
) *model.ClaimedStep {
	t.Helper()
	if claim == nil {
		t.Fatal("source recovery requires an initial claim")
	}
	terminal, err := pool.Exec(ctx, `
		UPDATE job_step_attempts SET status='expired',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID)
	if err != nil || terminal.RowsAffected() != 1 {
		t.Fatalf("expire attempt rows=%d err=%v", terminal.RowsAffected(), err)
	}
	nextAttempt := claim.Authority.Attempt + 1
	inserted, err := pool.Exec(ctx, `
		INSERT INTO job_step_attempts (
			job_id,generation,step_id,attempt,worker_id,claimed_at,renewed_at
		) VALUES ($1,$2,$3,$4,$5,clock_timestamp(),clock_timestamp())
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		nextAttempt, workerID)
	if err != nil || inserted.RowsAffected() != 1 {
		t.Fatalf("insert reclaimed attempt rows=%d err=%v", inserted.RowsAffected(), err)
	}
	advanced, err := pool.Exec(ctx, `
		UPDATE job_steps SET worker_id=$4,current_attempt=$3,updated_at=clock_timestamp()
		WHERE id=$1 AND job_id=$2 AND status='running'
	`, claim.Authority.StepID, claim.Authority.JobID, nextAttempt, workerID)
	if err != nil || advanced.RowsAffected() != 1 {
		t.Fatalf("advance reclaimed step rows=%d err=%v", advanced.RowsAffected(), err)
	}
	recovered := *claim
	recovered.Authority = model.StepAttemptAuthority{
		JobID: claim.Authority.JobID, Generation: claim.Authority.Generation,
		StepID: claim.Authority.StepID, Attempt: nextAttempt, WorkerID: workerID,
	}
	recovered.Step.WorkerID = workerID
	recovered.LeaseDeadline = time.Now().Add(time.Minute)
	return &recovered
}
