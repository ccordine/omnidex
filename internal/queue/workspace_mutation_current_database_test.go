package queue

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestPostgresCurrentWorkspaceMutationReturnsExactVerifiedTerminal(t *testing.T) {
	fixture := newWorkspaceMutationDatabaseFixture(t, "current-verified")
	var calls workspaceMutationCallbackCounts
	result, err := fixture.repository.ExecuteWorkspaceMutation(
		fixture.ctx, fixture.authority, fixture.command,
		workspaceMutationFixtureCallbacks(fixture, true, &calls),
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := fixture.repository.CurrentWorkspaceMutation(
		fixture.ctx, fixture.command.JobID, fixture.command.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertCurrentWorkspaceMutationTerminal(t, snapshot, fixture.command, result, "")

	if _, err := fixture.repository.ReplanJob(fixture.ctx, testReplanCommand(
		t, fixture.command.JobID, "current-workspace-stale",
		"Advance beyond the sealed workspace mutation generation.",
	)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.CurrentWorkspaceMutation(
		fixture.ctx, fixture.command.JobID, fixture.command.Generation,
	); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("stale current workspace mutation error=%v, want ErrStaleStepAttempt", err)
	}
}

func TestPostgresCurrentWorkspaceMutationReturnsExactFailedTerminalAndRejectsDuplicates(t *testing.T) {
	fixture := newWorkspaceMutationDatabaseFixture(t, "current-failed")
	var calls workspaceMutationCallbackCounts
	result, err := fixture.repository.ExecuteWorkspaceMutation(
		fixture.ctx, fixture.authority, fixture.command,
		workspaceMutationFixtureCallbacks(fixture, false, &calls),
	)
	if err != nil {
		t.Fatal(err)
	}

	snapshot, err := fixture.repository.CurrentWorkspaceMutation(
		fixture.ctx, fixture.command.JobID, fixture.command.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	assertCurrentWorkspaceMutationTerminal(
		t, snapshot, fixture.command, result, "generated file verification failed",
	)

	executeSecondWorkspaceMutationTerminal(t, fixture)
	if _, err := fixture.repository.CurrentWorkspaceMutation(
		fixture.ctx, fixture.command.JobID, fixture.command.Generation,
	); !errors.Is(err, ErrWorkspaceMutationConflict) {
		t.Fatalf("duplicate current workspace mutation error=%v, want ErrWorkspaceMutationConflict", err)
	}
}

func TestPostgresCurrentWorkspaceMutationReturnsNonterminalWithoutTerminalAuthority(t *testing.T) {
	fixture := newWorkspaceMutationDatabaseFixture(t, "current-nonterminal")

	missing, err := fixture.repository.CurrentWorkspaceMutation(
		fixture.ctx, fixture.command.JobID, fixture.command.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if missing != nil {
		t.Fatalf("missing current workspace mutation=%+v", missing)
	}

	identity, err := workspaceMutationOperation(fixture.command)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.repository.prepareWorkspaceMutation(
		fixture.ctx, fixture.authority, fixture.command, identity,
	); err != nil {
		t.Fatal(err)
	}

	snapshot, err := fixture.repository.CurrentWorkspaceMutation(
		fixture.ctx, fixture.command.JobID, fixture.command.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot == nil || snapshot.OperationID != identity.ID ||
		snapshot.Status != workspaceMutationPrepared || snapshot.Terminal != nil ||
		!reflect.DeepEqual(snapshot.Command, fixture.command) {
		t.Fatalf("nonterminal current workspace mutation=%+v", snapshot)
	}
}

func assertCurrentWorkspaceMutationTerminal(
	t *testing.T,
	snapshot *WorkspaceMutationSnapshot,
	command WorkspaceMutationCommand,
	result WorkspaceMutationResult,
	wantFailure string,
) {
	t.Helper()
	if snapshot == nil || snapshot.OperationID != result.OperationID ||
		snapshot.Status != result.Status || !reflect.DeepEqual(snapshot.Command, command) ||
		snapshot.Terminal == nil || !reflect.DeepEqual(snapshot.Terminal.Result, result) ||
		snapshot.Terminal.Failure != wantFailure ||
		len(snapshot.Terminal.ReceiptSHA256) != 64 || snapshot.Terminal.ReceiptJSON == "" {
		t.Fatalf("current terminal workspace mutation=%+v want result=%+v failure=%q", snapshot, result, wantFailure)
	}
	if digestWorkspaceMutationText(snapshot.Terminal.ReceiptJSON) != snapshot.Terminal.ReceiptSHA256 {
		t.Fatalf("current terminal receipt digest=%q", snapshot.Terminal.ReceiptSHA256)
	}
	var receipt workspaceMutationVerificationReceipt
	if err := json.Unmarshal([]byte(snapshot.Terminal.ReceiptJSON), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Schema != workspaceMutationReceiptSchema ||
		receipt.OperationID != result.OperationID ||
		receipt.SourceStateID != command.Plan.SourceStateID ||
		receipt.ExpectedStateID != command.Plan.ExpectedStateID ||
		receipt.ObservedStateID != command.Plan.ExpectedStateID ||
		receipt.Succeeded != result.VerificationSucceeded ||
		!reflect.DeepEqual(receipt.CommandEvidenceIDs, result.CommandEvidenceIDs) {
		t.Fatalf("current terminal workspace mutation receipt=%+v", receipt)
	}
}

func executeSecondWorkspaceMutationTerminal(
	t *testing.T,
	fixture workspaceMutationDatabaseFixture,
) {
	t.Helper()
	source, err := workspacefacts.Capture(fixture.ctx, fixture.root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspacefacts.PlanMutation(
		fixture.ctx, source,
		"objective_"+queueTestSHA256("workspace-current-second"),
		[]workspacefacts.DesiredFileState{{
			Path: "generated/second.txt", Present: true,
			Content: []byte("second terminal mutation\n"), Mode: 0o644,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspacefacts.StageMutation(fixture.ctx, source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stage.Cleanup(); err != nil {
			t.Error(err)
		}
	})
	verificationCommand := "verify generated/second.txt"
	verification, err := NewWorkspaceMutationVerificationPlan(
		[]WorkspaceMutationVerificationIntent{{
			Kind: evidence.KindTestResult, Command: verificationCommand,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	command := fixture.command
	command.Plan = plan
	command.Verification = verification
	second := fixture
	second.command = command
	second.stage = stage
	second.verificationCommand = verificationCommand
	var calls workspaceMutationCallbackCounts
	if _, err := fixture.repository.ExecuteWorkspaceMutation(
		fixture.ctx, fixture.authority, command,
		workspaceMutationFixtureCallbacks(second, true, &calls),
	); err != nil {
		t.Fatal(err)
	}
}
