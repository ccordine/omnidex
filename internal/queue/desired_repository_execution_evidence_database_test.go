package queue

import (
	"reflect"
	"strings"
	"testing"

	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestPostgresGitWorkspaceMutationReturnsVerifiedSnapshotAcrossCurrentAndReplay(t *testing.T) {
	fixture := newDesiredExecutionFixture(t, "snapshot-result-authority", "modify")
	snapshot, err := fixture.repository.CurrentWorkspaceMutation(
		fixture.ctx, fixture.command.JobID, fixture.command.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil || snapshot.Terminal == nil ||
		snapshot.Terminal.Result.VerifiedRepositorySnapshotID != fixture.after.ID {
		t.Fatalf("current Git workspace mutation snapshot authority=%+v want %q", snapshot, fixture.after.ID)
	}

	var calls workspaceMutationCallbackCounts
	replayed, err := fixture.repository.ExecuteWorkspaceMutation(
		fixture.ctx, fixture.authority, fixture.command,
		workspaceMutationForbiddenReplayCallbacks(&calls),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(replayed, snapshot.Terminal.Result) ||
		replayed.VerifiedRepositorySnapshotID != fixture.after.ID {
		t.Fatalf("replayed Git workspace mutation=%+v want %+v", replayed, snapshot.Terminal.Result)
	}
	if calls != (workspaceMutationCallbackCounts{}) {
		t.Fatalf("terminal Git replay ran callbacks=%+v", calls)
	}
}

func TestPostgresDesiredRepositoryExecutionEvidenceCountsExactDurableTransitions(t *testing.T) {
	for _, test := range []struct {
		name                       string
		created, deleted, modified int
		delta                      int
	}{
		{name: "create", created: 1, delta: 1},
		{name: "modify", modified: 1},
		{name: "delete", deleted: 1, delta: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newDesiredExecutionFixture(t, "exact-"+test.name, test.name)
			fixture.recordVerification(t)
			fixture.recordPostIndex(t)
			proof, err := fixture.repository.DesiredRepositoryExecutionEvidence(
				fixture.ctx, fixture.authority, fixture.graphID,
				fixture.before.ID, fixture.after.ID,
			)
			if err != nil {
				t.Fatal(err)
			}
			if proof.MutationOperations != 1 || proof.FileTransitions != 1 ||
				proof.CreatedFiles != test.created || proof.DeletedFiles != test.deleted ||
				proof.ModifiedFiles != test.modified || proof.InventoryDelta != test.delta ||
				proof.VerificationCommands.Baseline != 1 ||
				proof.VerificationCommands.Staged != 1 ||
				proof.VerificationCommands.Authoritative != 1 ||
				proof.PostStateRepositoryReindexes != 1 || proof.DeterministicOperations() != 5 {
				t.Fatalf("desired execution proof=%+v", proof)
			}
		})
	}
}

func TestPostgresDesiredRepositoryExecutionEvidenceRejectsIncompleteAndDuplicateProof(t *testing.T) {
	fixture := newDesiredExecutionFixture(t, "proof-closure", "modify")
	_, err := fixture.repository.DesiredRepositoryExecutionEvidence(
		fixture.ctx, fixture.authority, fixture.graphID, fixture.before.ID, fixture.after.ID,
	)
	if err == nil || !strings.Contains(err.Error(), "verification scope") {
		t.Fatalf("missing verification error=%v", err)
	}
	fixture.recordVerification(t)
	_, err = fixture.repository.DesiredRepositoryExecutionEvidence(
		fixture.ctx, fixture.authority, fixture.graphID, fixture.before.ID, fixture.after.ID,
	)
	if err == nil || !strings.Contains(err.Error(), "post-state repository reindex") {
		t.Fatalf("missing reindex error=%v", err)
	}
	fixture.recordPostIndex(t)
	if _, err := fixture.repository.DesiredRepositoryExecutionEvidence(
		fixture.ctx, fixture.authority, fixture.graphID, fixture.before.ID, fixture.after.ID,
	); err != nil {
		t.Fatal(err)
	}
	fixture.recordPostIndex(t)
	_, err = fixture.repository.DesiredRepositoryExecutionEvidence(
		fixture.ctx, fixture.authority, fixture.graphID, fixture.before.ID, fixture.after.ID,
	)
	if err == nil || !strings.Contains(err.Error(), "one exact post-state repository reindex") {
		t.Fatalf("duplicate reindex error=%v", err)
	}
}

func TestPostgresDesiredRepositoryExecutionEvidenceRejectsWrongAuthorityAndDuplicateMutation(t *testing.T) {
	fixture := newDesiredExecutionFixture(t, "authority", "modify")
	fixture.recordVerification(t)
	fixture.recordPostIndex(t)
	wrong := fixture.authority
	wrong.WorkerID += "-stale"
	if _, err := fixture.repository.DesiredRepositoryExecutionEvidence(
		fixture.ctx, wrong, fixture.graphID, fixture.before.ID, fixture.after.ID,
	); err == nil {
		t.Fatal("stale attempt read desired execution evidence")
	}
	wrongGraph := "desired_graph_" + queueTestSHA256("wrong graph")
	_, err := fixture.repository.DesiredRepositoryExecutionEvidence(
		fixture.ctx, fixture.authority, wrongGraph, fixture.before.ID, fixture.after.ID,
	)
	if err == nil || !strings.Contains(err.Error(), "found 0") {
		t.Fatalf("wrong graph error=%v", err)
	}

	source, err := workspacefacts.Capture(fixture.ctx, fixture.command.Plan.WorkspaceRoot)
	if err != nil {
		t.Fatal(err)
	}
	desired := desiredExecutionState(t, source, "modify", []byte("duplicate-post"))
	duplicate, err := workspacefacts.PlanMutation(
		fixture.ctx, source, fixture.graphID, []workspacefacts.DesiredFileState{desired},
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.executeMutation(t, source, duplicate)
	_, err = fixture.repository.DesiredRepositoryExecutionEvidence(
		fixture.ctx, fixture.authority, fixture.graphID, fixture.before.ID, fixture.after.ID,
	)
	if err == nil || !strings.Contains(err.Error(), "found 2") {
		t.Fatalf("duplicate mutation error=%v", err)
	}
}
