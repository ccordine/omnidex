package queue

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPostgresCognitionEnvironmentRejectsReceiptWithoutJournalAdvance(t *testing.T) {
	fixture, episode, action, receipt := environmentFailureGuardFixture(t, "receipt-only")
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := insertCognitionEnvironmentReceiptTx(
		fixture.Context, tx, episode, receipt,
	); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.Context); err == nil {
		t.Fatal("post-commit failure receipt without its journal projection was accepted")
	}
	_ = action
}

func TestPostgresCognitionEnvironmentRejectsJournalAdvanceWithoutReceipt(t *testing.T) {
	fixture, episode, _, receipt := environmentFailureGuardFixture(t, "journal-only")
	raw, sha, err := cognitionJSON(receipt)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(fixture.Context, `
		UPDATE cognition_environment_journals
		SET commit_sequence=1,last_receipt_json=$2,last_receipt_sha256=$3,
		    updated_at=clock_timestamp()
		WHERE episode_id=$1
	`, episode.ID, string(raw), sha); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(fixture.Context); err == nil {
		t.Fatal("journal-only environment advance without its exact receipt was accepted")
	}
}

func environmentFailureGuardFixture(
	t *testing.T,
	label string,
) (taskGenerationRetirementFixture, cognition.EpisodeRef, CognitionActionRecord, cognition.EnvironmentReceipt) {
	t.Helper()
	fixture := startTaskGenerationRetirementFixture(t, "environment-guard-"+label)
	episode := cognition.EpisodeRef{ID: fixture.EpisodeID}
	if _, err := fixture.Repository.StartCognitionEnvironment(
		fixture.Context, episode, fixture.Start.Scenario, fixture.Start.Transition,
	); err != nil {
		t.Fatal(err)
	}
	action := prepareCognitionGuardAction(t, fixture, "environment-guard-"+label)
	failure, err := cognition.NewActionFailure(
		cognition.ActionFailurePreconditionFailed, action.Action, action.ExpectedRevision,
		"The registered action precondition was not satisfied.", []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := cognition.NewEnvironmentFailureReceipt(
		episode, action.Action, action.ExpectedRevision, failure,
	)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, episode, action, receipt
}
