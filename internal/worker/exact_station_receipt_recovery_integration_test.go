package worker

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestFreshSchemaExpiredSuccessfulReceiptIsValidatedAndAcceptedWithoutInference(t *testing.T) {
	pool, repository, ctx, claim1 := freshInterruptedRecoveryClaim(t, "receipt-accepted")
	input := assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	client1 := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{candidate: "A"}}}
	runtime1 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: &Service{
			repo: repository, stationClient: client1, inferenceContextTokens: "8192",
			runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
		},
		ctx: ctx, claim: claim1,
	}, "receipt-accepted-1")
	initial, err := runtime1.Execute(job, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	if initial.Candidate != "A" || client1.calls != 1 {
		t.Fatalf("initial result=%#v provider calls=%d", initial, client1.calls)
	}

	claim2 := reclaimEvidenceAttemptForTest(t, ctx, pool, claim1, "receipt-accepted-2")
	if _, err := setWorkerEvidenceAttemptStatus(ctx, pool, claim2.Authority, "completed"); err == nil {
		t.Fatal("successor completed while its predecessor provider receipt was unconsumed")
	}
	client2 := &exactEvidenceStationClient{}
	runtime2 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: &Service{
			repo: repository, stationClient: client2, inferenceContextTokens: "16384",
			runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
		},
		ctx: ctx, claim: claim2,
	}, "receipt-accepted-2")
	recovered, err := runtime2.Execute(job, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	decoded, validationErr := assemblyline.DecodeApplicationClassification(input, recovered.Candidate)
	if validationErr != nil {
		t.Fatal(validationErr)
	}
	if decoded.Surface != assemblyline.ApplicationSurfaceBrowser {
		t.Fatalf("decoded surface=%q", decoded.Surface)
	}
	if err := runtime2.Finalize(job, recovered, nil); err != nil {
		t.Fatal(err)
	}
	if client2.calls != 0 || runtime2.ProviderCalls == nil || runtime2.ProviderCalls() != 0 {
		t.Fatalf("receipt recovery invoked provider calls=%d", client2.calls)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, claim1.Job.ID)
	if err != nil || len(calls) != 1 || calls[0].StepAttempt != claim1.Authority.Attempt ||
		calls[0].Outcome == nil || calls[0].Outcome.Status != queue.LLMCallAccepted {
		t.Fatalf("accepted recovered receipt=%#v err=%v", calls, err)
	}
	if _, err := setWorkerEvidenceAttemptStatus(ctx, pool, claim2.Authority, "completed"); err != nil {
		t.Fatalf("successor could not complete after consuming predecessor receipt: %v", err)
	}
}

func TestFreshSchemaExpiredSuccessfulReceiptIsValidatedAndRejectedWithoutInference(t *testing.T) {
	pool, repository, ctx, claim1 := freshInterruptedRecoveryClaim(t, "receipt-rejected")
	input := assemblyline.ApplicationClassificationInput{UserRequest: "classify one interface"}
	job, err := assemblyline.NewApplicationClassificationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	client1 := &exactEvidenceStationClient{fixtures: []exactEvidenceStationFixture{{
		candidate: "ordinary prose is not an opaque choice",
	}}}
	runtime1 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: &Service{
			repo: repository, stationClient: client1, inferenceContextTokens: "8192",
			runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
		},
		ctx: ctx, claim: claim1,
	}, "receipt-rejected-1")
	initial, err := runtime1.Execute(job, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}

	claim2 := reclaimEvidenceAttemptForTest(t, ctx, pool, claim1, "receipt-rejected-2")
	client2 := &exactEvidenceStationClient{}
	runtime2 := portableWorkerRuntime(&nativeRuntimeV3{
		svc: &Service{
			repo: repository, stationClient: client2, inferenceContextTokens: "16384",
			runtimeEventChannels: make(map[int64]runtimeEventChannelBinding),
		},
		ctx: ctx, claim: claim2,
	}, "receipt-rejected-2")
	recovered, err := runtime2.Execute(job, "fixture-model")
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Candidate != initial.Candidate {
		t.Fatalf("recovered candidate=%q want=%q", recovered.Candidate, initial.Candidate)
	}
	_, validationErr := assemblyline.DecodeApplicationClassification(input, recovered.Candidate)
	if validationErr == nil {
		t.Fatal("invalid recovered response unexpectedly decoded")
	}
	if err := runtime2.Finalize(job, recovered, validationErr); err != nil {
		t.Fatal(err)
	}
	if client2.calls != 0 || runtime2.ProviderCalls == nil || runtime2.ProviderCalls() != 0 {
		t.Fatalf("rejected receipt recovery invoked provider calls=%d", client2.calls)
	}
	calls, err := listAllWorkerLLMCallEvidence(ctx, repository, claim1.Job.ID)
	if err != nil || len(calls) != 1 || calls[0].StepAttempt != claim1.Authority.Attempt ||
		calls[0].Outcome == nil || calls[0].Outcome.Status != queue.LLMCallRejected ||
		calls[0].Outcome.ValidationError != exactStationEvidenceError(validationErr) {
		t.Fatalf("rejected recovered receipt=%#v err=%v", calls, err)
	}
}

func setWorkerEvidenceAttemptStatus(
	ctx context.Context,
	pool *pgxpool.Pool,
	authority model.StepAttemptAuthority,
	status string,
) (pgconn.CommandTag, error) {
	return pool.Exec(ctx, `
		UPDATE job_step_attempts SET status=$6,finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, status)
}
