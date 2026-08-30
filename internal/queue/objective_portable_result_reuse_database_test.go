package queue

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

const roleplayPortableReuseExactCandidate = "The accepted first responder leaf."

func TestPostgresObjectivePortableResultReuseRestoresOnlySameNonRoleplayJob(t *testing.T) {
	ctx, repository, pool := openWorkingSetDatabase(t)
	sourceJob := enqueueWorkingSetTestJob(t, ctx, repository, "objective-reuse-same-job")
	sourceAuthority := claimWorkingSetTestJob(t, ctx, repository, sourceJob)
	root, err := assemblyline.NewGroundedAnswerParagraphInventoryJob(
		assemblyline.GroundedAnswerParagraphInventoryInput{
			ExactRequirement: "Which component owns the lease?",
			Evidence: []assemblyline.GroundedEvidenceCapsule{{
				ID: "registry", Text: "LeaseRegistry owns the lease.",
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	const candidate = "LeaseRegistry owns the lease."
	sourceClaim := &model.ClaimedStep{Authority: sourceAuthority}
	persistObjectivePortableReuseLeaf(t, repository, sourceClaim, root, candidate)
	targetAuthority := replaceStepAttemptForTest(t, pool, sourceAuthority)

	reused, found, err := repository.ReuseObjectivePortableResult(
		ctx,
		ObjectivePortableResultReuseRequest{
			Authority: targetAuthority, Job: root,
			Station: objectivePortableReuseStation(t, root),
		},
	)
	if err != nil || !found {
		t.Fatalf("same-job reuse found=%t err=%v", found, err)
	}
	if reused.Result.Candidate != candidate ||
		reused.Receipt.SourceAuthority.JobID != sourceJob.ID ||
		reused.Receipt.SourceAuthority.Attempt != sourceAuthority.Attempt {
		t.Fatalf("same-job reuse=%+v", reused)
	}

	if err := repository.FailStep(ctx, FailStepCommand{
		OperationID: testLifecycleOperationID(t, "objective-reuse-source-failed", targetAuthority.StepID),
		Authority:   targetAuthority, StepID: targetAuthority.StepID, Error: "injected later failure",
	}); err != nil {
		t.Fatal(err)
	}
	otherJob := enqueueWorkingSetTestJob(t, ctx, repository, "objective-reuse-other-job")
	otherAuthority := claimWorkingSetTestJob(t, ctx, repository, otherJob)
	if _, found, err := repository.ReuseObjectivePortableResult(
		ctx,
		ObjectivePortableResultReuseRequest{
			Authority: otherAuthority, Job: root,
			Station: objectivePortableReuseStation(t, root),
		},
	); err != nil || found {
		t.Fatalf("cross-job non-roleplay reuse found=%t err=%v", found, err)
	}
}

func TestPostgresObjectivePortableResultReusePreservesFailedAcceptedLeaf(t *testing.T) {
	fixture := newRoleplayPortableReuseDatabaseFixture(t, "roleplay-reuse-failed")
	_, sourceJob, err := enqueueNarratorRoleplayTurn(
		t.Context(), fixture.Repository, fixture.Channel.ID, "Hold the line.",
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceClaim, err := fixture.Repository.ClaimNextStep(t.Context(), "roleplay-reuse-source")
	if err != nil || sourceClaim == nil || sourceClaim.Job.ID != sourceJob.ID {
		t.Fatalf("source claim=%+v err=%v", sourceClaim, err)
	}
	if respondersInRoleplayReuseJob(t, sourceJob) != 2 {
		t.Fatal("source job does not have the required two-responder round")
	}
	root := roleplayPortableReuseRootJob(t, "resolve the bounded roleplay leaf")
	sourceOutcome := persistObjectivePortableReuseLeaf(
		t, fixture.Repository, sourceClaim, root, roleplayPortableReuseExactCandidate,
	)
	failRoleplayPortableReuseJob(t, fixture.Repository, sourceClaim, "roleplay-reuse-source-fail")

	// A generation-model revision is deliberately outside fictional reuse
	// authority. Character IDs and narrative fingerprints remain exact.
	for _, characterID := range fixture.CharacterIDs {
		current, err := fixture.Store.ProjectCharacterGeneration(
			t.Context(), fixture.WorldID, characterID,
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Store.WriteCharacterGeneration(t.Context(), roleplay.CharacterGenerationWriteRequest{
			WorldID: fixture.WorldID, CharacterID: characterID,
			ExpectedRevision: current.Config.Revision, NarrativeModel: "qwen3.5:9b",
		}); err != nil {
			t.Fatal(err)
		}
	}
	_, targetJob, err := enqueueNarratorRoleplayTurn(
		t.Context(), fixture.Repository, fixture.Channel.ID, "Hold the line.",
	)
	if err != nil {
		t.Fatal(err)
	}
	assertRoleplayReuseGenerationChangedOnly(t, sourceJob, targetJob)
	targetClaim, err := fixture.Repository.ClaimNextStep(t.Context(), "roleplay-reuse-target")
	if err != nil || targetClaim == nil || targetClaim.Job.ID != targetJob.ID {
		t.Fatalf("target claim=%+v err=%v", targetClaim, err)
	}
	request := ObjectivePortableResultReuseRequest{
		Authority: targetClaim.Authority, Job: root,
		Station: objectivePortableReuseStation(t, root),
	}
	reused, found, err := fixture.Repository.ReuseObjectivePortableResult(t.Context(), request)
	if err != nil || !found {
		t.Fatalf("reuse found=%t err=%v", found, err)
	}
	if reused.Result.JobID != root.ID || reused.Result.Candidate != roleplayPortableReuseExactCandidate ||
		reused.Receipt.SourceAuthority.JobID != sourceJob.ID ||
		reused.Receipt.SourceGapOutcomeID != sourceOutcome.ID ||
		reused.Receipt.SourceResponseSHA256 != sourceOutcome.ResponseSHA256 {
		t.Fatalf("reuse=%+v", reused)
	}
	if err := reused.Result.ValidateFor(root); err != nil {
		t.Fatalf("reused result: %v", err)
	}

	duplicate, duplicateFound, err := fixture.Repository.ReuseObjectivePortableResult(t.Context(), request)
	if err != nil || !duplicateFound || duplicate.Receipt.ID != reused.Receipt.ID ||
		duplicate.Result.Candidate != reused.Result.Candidate {
		t.Fatalf("idempotent reuse=%+v found=%t err=%v", duplicate, duplicateFound, err)
	}
	var receiptCount int
	if err := fixture.Pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM objective_portable_result_reuses
		WHERE target_job_id=$1 AND target_step_attempt=$2
	`, targetJob.ID, targetClaim.Authority.Attempt).Scan(&receiptCount); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 1 {
		t.Fatalf("idempotent reuse receipts=%d want 1", receiptCount)
	}

	changedWork := roleplayPortableReuseRootJob(t, "a different bounded leaf")
	if _, found, err := fixture.Repository.ReuseObjectivePortableResult(t.Context(),
		ObjectivePortableResultReuseRequest{
			Authority: targetClaim.Authority, Job: changedWork,
			Station: objectivePortableReuseStation(t, changedWork),
		}); err != nil || found {
		t.Fatalf("changed work found=%t err=%v", found, err)
	}
}

func TestPostgresObjectivePortableResultReuseRejectsFictionalAuthorityDrift(t *testing.T) {
	t.Run("user turn", func(t *testing.T) {
		fixture := newRoleplayPortableReuseDatabaseFixture(t, "roleplay-reuse-user-drift")
		root := roleplayPortableReuseRootJob(t, "authority-sensitive leaf")
		seedFailedRoleplayPortableReuseSource(t, fixture, "Wait here.", root)
		_, targetJob, err := enqueueNarratorRoleplayTurn(
			t.Context(), fixture.Repository, fixture.Channel.ID, "Move now.",
		)
		if err != nil {
			t.Fatal(err)
		}
		assertRoleplayPortableReuseAbsent(t, fixture.Repository, targetJob, root, "user-drift-target")
	})

	t.Run("scene revision", func(t *testing.T) {
		fixture := newRoleplayPortableReuseDatabaseFixture(t, "roleplay-reuse-scene-drift")
		root := roleplayPortableReuseRootJob(t, "scene-sensitive leaf")
		seedFailedRoleplayPortableReuseSource(t, fixture, "Wait here.", root)
		_, bridge, err := enqueueNarratorRoleplayTurn(
			t.Context(), fixture.Repository, fixture.Channel.ID, "Time passes.",
		)
		if err != nil {
			t.Fatal(err)
		}
		bridgeClaim, err := fixture.Repository.ClaimNextStep(t.Context(), "scene-drift-bridge")
		if err != nil || bridgeClaim == nil || bridgeClaim.Job.ID != bridge.ID {
			t.Fatalf("bridge claim=%+v err=%v", bridgeClaim, err)
		}
		completeRoleplayPortableReuseJob(t, fixture.Repository, bridgeClaim, "scene-drift-bridge-done")
		_, targetJob, err := enqueueNarratorRoleplayTurn(
			t.Context(), fixture.Repository, fixture.Channel.ID, "Wait here.",
		)
		if err != nil {
			t.Fatal(err)
		}
		assertRoleplayPortableReuseAbsent(t, fixture.Repository, targetJob, root, "scene-drift-target")
	})
}

func TestPostgresObjectivePortableResultReuseExcludesCompletedTurn(t *testing.T) {
	fixture := newRoleplayPortableReuseDatabaseFixture(t, "roleplay-reuse-completed")
	_, _, err := enqueueNarratorRoleplayTurn(
		t.Context(), fixture.Repository, fixture.Channel.ID, "Complete this turn.",
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceClaim, err := fixture.Repository.ClaimNextStep(t.Context(), "completed-reuse-source")
	if err != nil || sourceClaim == nil {
		t.Fatalf("source claim=%+v err=%v", sourceClaim, err)
	}
	root := roleplayPortableReuseRootJob(t, "completed source leaf")
	persistObjectivePortableReuseLeaf(
		t, fixture.Repository, sourceClaim, root, roleplayPortableReuseExactCandidate,
	)
	completeRoleplayPortableReuseJob(t, fixture.Repository, sourceClaim, "completed-reuse-source-done")

	_, targetJob, err := enqueueNarratorRoleplayTurn(
		t.Context(), fixture.Repository, fixture.Channel.ID, "Complete this turn.",
	)
	if err != nil {
		t.Fatal(err)
	}
	targetClaim, err := fixture.Repository.ClaimNextStep(t.Context(), "completed-reuse-target")
	if err != nil || targetClaim == nil || targetClaim.Job.ID != targetJob.ID {
		t.Fatalf("target claim=%+v err=%v", targetClaim, err)
	}
	request := ObjectivePortableResultReuseRequest{
		Authority: targetClaim.Authority, Job: root,
		Station: objectivePortableReuseStation(t, root),
	}
	tx, err := fixture.Pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	candidates, listErr := listObjectivePortableReuseCandidatesTx(
		t.Context(), tx, request, targetJob,
	)
	_ = tx.Rollback(t.Context())
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(candidates) != 0 {
		t.Fatalf("completed source produced %d reuse candidates", len(candidates))
	}
	if _, found, err := fixture.Repository.ReuseObjectivePortableResult(
		t.Context(), request,
	); err != nil || found {
		t.Fatalf("completed source reuse found=%t err=%v", found, err)
	}
}

func TestPostgresObjectivePortableResultReuseExcludesCurrentAttemptAndFailedOutcome(t *testing.T) {
	fixture := newRoleplayPortableReuseDatabaseFixture(t, "roleplay-reuse-negative-outcomes")
	_, sourceJob, err := enqueueNarratorRoleplayTurn(
		t.Context(), fixture.Repository, fixture.Channel.ID, "Preserve only accepted work.",
	)
	if err != nil {
		t.Fatal(err)
	}
	sourceClaim, err := fixture.Repository.ClaimNextStep(t.Context(), "negative-reuse-source")
	if err != nil || sourceClaim == nil || sourceClaim.Job.ID != sourceJob.ID {
		t.Fatalf("source claim=%+v err=%v", sourceClaim, err)
	}
	resolvedRoot := roleplayPortableReuseRootJob(t, "current attempt leaf")
	persistObjectivePortableReuseLeaf(
		t, fixture.Repository, sourceClaim, resolvedRoot, roleplayPortableReuseExactCandidate,
	)
	if _, found, err := fixture.Repository.ReuseObjectivePortableResult(t.Context(),
		ObjectivePortableResultReuseRequest{
			Authority: sourceClaim.Authority, Job: resolvedRoot,
			Station: objectivePortableReuseStation(t, resolvedRoot),
		}); err != nil || found {
		t.Fatalf("current-attempt reuse found=%t err=%v", found, err)
	}

	failedRoot := roleplayPortableReuseRootJob(t, "failed outcome leaf")
	failedStation := objectivePortableReuseStation(t, failedRoot)
	const contextTokens = 32768
	failedGap, err := fixture.Repository.OpenStationGap(t.Context(), StationGapOpenRecord{
		Authority: sourceClaim.Authority, Job: failedRoot, Station: failedStation,
		ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(
			t, failedRoot, contextTokens,
		),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err != nil {
		t.Fatal(err)
	}
	persistStationDiscoveryFailure(t, fixture.Repository, sourceClaim.Authority, failedGap)
	if _, err := fixture.Repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: sourceClaim.Authority, OpeningID: failedGap.ID, GapID: failedGap.GapID,
		Status: StationGapFailed, Error: "provider discovery failed",
	}); err != nil {
		t.Fatal(err)
	}
	rejectedRoot := roleplayPortableReuseRootJob(t, "deterministically rejected candidate leaf")
	rejectedOutcome := persistRejectedObjectivePortableReuseLeaf(
		t, fixture.Repository, sourceClaim, rejectedRoot,
		`{"schema":"omnidex.conversation-response.v1","text":""}`,
	)
	if rejectedOutcome.Status != StationGapFailed || rejectedOutcome.Response != "" {
		t.Fatalf("rejected outcome=%+v", rejectedOutcome)
	}
	failRoleplayPortableReuseJob(t, fixture.Repository, sourceClaim, "negative-reuse-source-fail")
	_, targetJob, err := enqueueNarratorRoleplayTurn(
		t.Context(), fixture.Repository, fixture.Channel.ID, "Preserve only accepted work.",
	)
	if err != nil {
		t.Fatal(err)
	}
	targetClaim, err := fixture.Repository.ClaimNextStep(t.Context(), "negative-reuse-target")
	if err != nil || targetClaim == nil || targetClaim.Job.ID != targetJob.ID {
		t.Fatalf("target claim=%+v err=%v", targetClaim, err)
	}
	for label, ineligible := range map[string]assemblyline.PortableJob{
		"failed discovery":   failedRoot,
		"rejected candidate": rejectedRoot,
	} {
		if _, found, err := fixture.Repository.ReuseObjectivePortableResult(t.Context(),
			ObjectivePortableResultReuseRequest{
				Authority: targetClaim.Authority, Job: ineligible,
				Station: objectivePortableReuseStation(t, ineligible),
			}); err != nil || found {
			t.Fatalf("%s reuse found=%t err=%v", label, found, err)
		}
	}
}

func seedFailedRoleplayPortableReuseSource(
	t *testing.T,
	fixture roleplayPortableReuseDatabaseFixture,
	instruction string,
	root assemblyline.PortableJob,
) {
	t.Helper()
	_, job, err := enqueueNarratorRoleplayTurn(
		t.Context(), fixture.Repository, fixture.Channel.ID, instruction,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := fixture.Repository.ClaimNextStep(t.Context(), "authority-drift-source")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("source claim=%+v err=%v", claim, err)
	}
	persistObjectivePortableReuseLeaf(
		t, fixture.Repository, claim, root, roleplayPortableReuseExactCandidate,
	)
	failRoleplayPortableReuseJob(t, fixture.Repository, claim, "authority-drift-source-fail")
}

func assertRoleplayPortableReuseAbsent(
	t *testing.T,
	repository *Repository,
	targetJob model.Job,
	root assemblyline.PortableJob,
	worker string,
) {
	t.Helper()
	claim, err := repository.ClaimNextStep(t.Context(), worker)
	if err != nil || claim == nil || claim.Job.ID != targetJob.ID {
		t.Fatalf("target claim=%+v err=%v", claim, err)
	}
	if _, found, err := repository.ReuseObjectivePortableResult(t.Context(),
		ObjectivePortableResultReuseRequest{
			Authority: claim.Authority, Job: root,
			Station: objectivePortableReuseStation(t, root),
		}); err != nil || found {
		t.Fatalf("inexact authority reuse found=%t err=%v", found, err)
	}
}

func respondersInRoleplayReuseJob(t *testing.T, job model.Job) int {
	t.Helper()
	var metadata channelTurnMetadata
	if err := json.Unmarshal(job.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	return len(metadata.RoleplayResponders)
}

func assertRoleplayReuseGenerationChangedOnly(t *testing.T, source, target model.Job) {
	t.Helper()
	var before, after channelTurnMetadata
	if err := json.Unmarshal(source.Metadata, &before); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(target.Metadata, &after); err != nil {
		t.Fatal(err)
	}
	if len(before.RoleplayResponders) != len(after.RoleplayResponders) {
		t.Fatal("responder round changed")
	}
	for index := range before.RoleplayResponders {
		left, right := before.RoleplayResponders[index], after.RoleplayResponders[index]
		if left.CharacterID != right.CharacterID ||
			left.NarrativeFingerprint != right.NarrativeFingerprint {
			t.Fatalf("responder fictional authority changed: before=%+v after=%+v", left, right)
		}
		if left.GenerationConfig == right.GenerationConfig {
			t.Fatalf("responder %d generation config did not change", index)
		}
	}
}
