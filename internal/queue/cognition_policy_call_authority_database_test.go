package queue

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

type captureCognitionCallJournal struct {
	attempt cognitionpolicy.CallAttempt
}

func (journal *captureCognitionCallJournal) Start(
	_ context.Context,
	attempt cognitionpolicy.CallAttempt,
) (cognitionpolicy.CallReservation, error) {
	journal.attempt = attempt
	return cognitionpolicy.CallReservation{}, errors.New("capture exact call before persistence")
}

func (*captureCognitionCallJournal) Finish(
	context.Context,
	cognitionpolicy.CallAttempt,
	cognitionpolicy.CallResult,
	cognitionpolicy.CallEvidence,
) error {
	return errors.New("capture journal cannot finish")
}

func TestPostgresRejectsForgedCanonicalPolicyCallIdentity(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "forged-call-identity")
	attempt := capturePreparedCognitionCall(t, fixture)
	forged := attempt
	forged.ID = "cognition_call_" + strings.Repeat("f", 64)

	budgetJSON, budgetSHA, err := cognitionJSON(forged.RuntimeBudget)
	if err != nil {
		t.Fatal(err)
	}
	brainJSON, brainSHA, err := cognitionJSON(forged.Brain)
	if err != nil {
		t.Fatal(err)
	}
	attemptJSON, attemptSHA, err := cognitionJSON(forged)
	if err != nil {
		t.Fatal(err)
	}
	_, err = fixture.Pool.Exec(fixture.Context, `
		INSERT INTO cognition_policy_calls (
			call_id,episode_id,job_id,generation,step_id,step_attempt,worker_id,
			snapshot_sha256,projection_id,working_set_id,expected_revision,
			expected_revision_sha256,obligation_node_id,runtime_budget_json,runtime_budget_sha256,
			brain_json,brain_sha256,attempt_json,attempt_sha256,status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,'started')
	`, forged.ID, forged.ExpectedRevision.EpisodeID, fixture.Authority.JobID,
		fixture.Authority.Generation, fixture.Authority.StepID, fixture.Authority.Attempt,
		fixture.Authority.WorkerID, forged.SnapshotSHA256, forged.ContextProjection.ID,
		forged.ContextProjection.WorkingSetID, int64(forged.ExpectedRevision.Number),
		forged.ExpectedRevision.SHA256, forged.ObligationID, string(budgetJSON), budgetSHA,
		string(brainJSON), brainSHA, string(attemptJSON), attemptSHA)
	if err == nil {
		t.Fatal("direct SQL forged a noncanonical policy call identity")
	}
	var count int
	if err := fixture.Pool.QueryRow(fixture.Context,
		`SELECT COUNT(*) FROM cognition_policy_calls WHERE snapshot_sha256=$1`,
		attempt.SnapshotSHA256,
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("forged policy call occupied snapshot authority: count=%d", count)
	}
}

func capturePreparedCognitionCall(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
) cognitionpolicy.CallAttempt {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context,
		CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	journal := &captureCognitionCallJournal{}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: "unused"}, cognitionTestBrain(),
		cognitionGuardProjectionLoader{repository: fixture.Repository}, journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(fixture.Context, prepared.Prepared.Snapshot); err == nil {
		t.Fatal("capture policy unexpectedly completed")
	}
	if journal.attempt.ID == "" {
		t.Fatal("capture journal received no policy call")
	}
	return journal.attempt
}
