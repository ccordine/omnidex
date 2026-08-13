package queue

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresSemanticGapLifecycleIsExactImmutableAndSingular(t *testing.T) {
	repository, pool, claim := semanticGapTestClaim(t, "semantic-gap-lifecycle")
	opening := stationGapOpenFixture(t, claim.Authority)
	opened, err := repository.OpenStationGap(t.Context(), opening)
	if err != nil {
		t.Fatal(err)
	}
	prompt, _, renderErr := assemblyline.RenderPortableJob(opening.Job)
	if renderErr != nil {
		t.Fatal(renderErr)
	}
	if opened.GapID != opening.Job.ID || opened.Prompt != prompt || opened.ProjectionSHA256 == "" {
		t.Fatalf("opened event=%+v", opened)
	}
	if _, err := repository.OpenStationGap(t.Context(), opening); err == nil {
		t.Fatal("duplicate gap opening was accepted")
	}
	persistStationDiscoveryFailure(t, repository, claim.Authority, opened)
	resolved, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: opened.ID, GapID: opening.Job.ID,
		Status: StationGapFailed, Error: "exact provider discovery failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.OpeningID != opened.ID || resolved.Error != "exact provider discovery failed" {
		t.Fatalf("resolved event=%+v", resolved)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: opened.ID, GapID: opening.Job.ID,
		Status: StationGapFailed, Error: "second terminal",
	}); err == nil {
		t.Fatal("second terminal event was accepted")
	}
	if _, err := pool.Exec(t.Context(), `UPDATE station_gap_openings SET scope='forged' WHERE id=$1`, opened.ID); err == nil {
		t.Fatal("semantic-gap history allowed an update")
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM station_gap_outcomes WHERE id=$1`, resolved.ID); err == nil {
		t.Fatal("semantic-gap history allowed a delete")
	}
}

func TestPostgresCompleteStepRejectsOpenProviderDiscoveryAtomically(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "provider-discovery-completion")
	gapRecord := stationGapOpenFixture(t, claim.Authority)
	gapRecord.ContextTokens = 32768
	gap, err := repository.OpenStationGap(t.Context(), gapRecord)
	if err != nil {
		t.Fatal(err)
	}
	prepared := stationCallTestPrepared(t, gap)
	discovery, err := repository.OpenStationDiscovery(t.Context(), StationDiscoveryOpenRecord{
		Authority: claim.Authority, Gap: gap,
		Selection: llm.ProviderIdentitySelection{Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens},
	})
	if err != nil {
		t.Fatal(err)
	}
	command := CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "provider-discovery-open-completion", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Output: "must not commit",
	}
	if err := repository.CompleteStep(t.Context(), command); err == nil || !strings.Contains(err.Error(), "open provider discovery") {
		t.Fatalf("CompleteStep error=%v, want open-discovery rejection", err)
	}
	if _, err := repository.RecordStationDiscoveryReceipt(t.Context(), StationDiscoveryReceiptRecord{
		Authority: claim.Authority, OpeningID: discovery.ID, GapID: gap.GapID,
		Observed: llm.ObservedProviderIdentity{Evidence: stationCallIdentityFailure(t, prepared).ProviderIdentityEvidence},
		FailureReason: StationDiscoveryFailureEvidenceRejected,
		Error:         "provider discovery failed",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: gap.ID, GapID: gap.GapID,
		Status: StationGapFailed, Error: "provider discovery failed",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPostgresCompleteStepRejectsOpenSemanticGapAtomically(t *testing.T) {
	repository, _, claim := semanticGapTestClaim(t, "semantic-gap-completion")
	opening := stationGapOpenFixture(t, claim.Authority)
	opened, err := repository.OpenStationGap(t.Context(), opening)
	if err != nil {
		t.Fatal(err)
	}
	command := CompleteStepCommand{
		OperationID: testLifecycleOperationID(t, "semantic-gap-open-completion", claim.Step.ID),
		Authority:   claim.Authority, StepID: claim.Step.ID, Output: "must not commit",
	}
	if err := repository.CompleteStep(t.Context(), command); err == nil || !strings.Contains(err.Error(), "open semantic gap") {
		t.Fatalf("CompleteStep error=%v, want open-gap rejection", err)
	}
	details, err := repository.CurrentJobDetails(t.Context(), claim.Job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if details.Job.Status != model.JobStatusRunning || details.Steps[0].Status != model.StepStatusRunning {
		t.Fatalf("failed completion crossed transaction boundary: %+v", details)
	}
	persistStationDiscoveryFailure(t, repository, claim.Authority, opened)
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: opened.ID, GapID: opening.Job.ID,
		Status: StationGapFailed, Error: "bounded inference failed",
	}); err != nil {
		t.Fatal(err)
	}
	command.OperationID = testLifecycleOperationID(t, "semantic-gap-closed-completion", claim.Step.ID)
	if err := repository.CompleteStep(t.Context(), command); err != nil {
		t.Fatalf("closed gap blocked completion: %v", err)
	}
}

func semanticGapTestClaim(t *testing.T, marker string) (*Repository, *pgxpool.Pool, *model.ClaimedStep) {
	t.Helper()
	ctx := context.Background()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "080")); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "station-gap-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	return repository, pool, claim
}

func stationGapOpenFixture(t *testing.T, authority model.StepAttemptAuthority) StationGapOpenRecord {
	t.Helper()
	job, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "\n exact named semantic question \n",
	})
	if err != nil {
		t.Fatal(err)
	}
	return StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.ConversationResponse,
		ContextTokens: 32768, MaxOutputTokens: 1024,
	}
}

func persistStationDiscoveryFailure(
	t *testing.T,
	repository *Repository,
	authority model.StepAttemptAuthority,
	gap StationGapOpening,
) StationDiscoveryReceipt {
	t.Helper()
	prepared := stationCallTestPrepared(t, gap)
	selection := llm.ProviderIdentitySelection{Model: prepared.ContextModel, NativeContextLimit: prepared.ContextTokens}
	opening, err := repository.OpenStationDiscovery(t.Context(), StationDiscoveryOpenRecord{
		Authority: authority, Gap: gap, Selection: selection,
	})
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := repository.RecordStationDiscoveryReceipt(t.Context(), StationDiscoveryReceiptRecord{
		Authority: authority, OpeningID: opening.ID, GapID: gap.GapID,
		Observed: llm.ObservedProviderIdentity{Evidence: stationCallIdentityFailure(t, prepared).ProviderIdentityEvidence},
		FailureReason: StationDiscoveryFailureEvidenceRejected,
		Error:         "exact provider discovery failed",
	})
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}
