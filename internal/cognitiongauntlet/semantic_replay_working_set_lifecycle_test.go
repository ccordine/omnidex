package cognitiongauntlet

import (
	"testing"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func semanticWorkingSetLifecycle(t *testing.T) (
	*workingset.Set,
	workingset.Snapshot,
	[]workingset.Event,
) {
	t.Helper()
	ledgerID, err := taskstate.NewLedgerID(taskstate.LedgerOwner{
		Kind: taskstate.OwnerJob, JobID: 93, RunID: "23456789-abcd-ef01-2345-6789abcdef01",
	})
	if err != nil {
		t.Fatal(err)
	}
	set, err := workingset.New(workingset.Owner{LedgerID: ledgerID, JobID: 93, Generation: 1},
		workingset.Budget{MaxItems: 2, MaxBytes: 16, MaxPinnedItems: 1, MaxPinnedBytes: 8})
	if err != nil {
		t.Fatal(err)
	}
	start := set.Snapshot()
	taskScope := workingset.Scope{Kind: workingset.ScopeTask, ID: "task-lifecycle"}
	objectiveScope := workingset.Scope{Kind: workingset.ScopeObjective, ID: "objective-lifecycle"}
	commands := []workingset.Command{
		semanticAcquireCommand(t, set, "lifecycle-first", taskScope, 0),
		workingset.RetainCommand{
			CommandID: semanticWorkingCommandID(t, "retain"), ExpectedVersion: 1,
			Actor: taskstate.AuthorityCode, ItemID: "lifecycle-first",
			Scope: objectiveScope, Retention: workingset.RetentionObjective,
		},
		workingset.TouchCommand{
			CommandID: semanticWorkingCommandID(t, "touch"), ExpectedVersion: 2,
			Actor: taskstate.AuthorityCode, ItemIDs: []workingset.ItemID{"lifecycle-first"},
		},
		workingset.ReleaseCommand{
			CommandID: semanticWorkingCommandID(t, "release-membership"), ExpectedVersion: 3,
			Actor: taskstate.AuthorityCode, ItemID: "lifecycle-first", Scope: taskScope,
			Reason: "The task scope completed.",
		},
		workingset.InvalidateStaleCommand{
			CommandID: semanticWorkingCommandID(t, "invalidate"), ExpectedVersion: 4,
			Actor: taskstate.AuthorityCode, ItemID: "lifecycle-first",
			CurrentVersion: "2", CurrentHash: traceTestDigest("current-source"),
			Reason: "The exact source changed.",
		},
		semanticAcquireCommand(t, set, "lifecycle-second", taskScope, 5),
		workingset.CloseScopeCommand{
			CommandID: semanticWorkingCommandID(t, "close-scope"), ExpectedVersion: 6,
			Actor: taskstate.AuthorityCode, Scope: taskScope,
			Reason: "The second task scope completed.",
		},
	}
	events := make([]workingset.Event, len(commands))
	for index, command := range commands {
		events[index], err = set.Apply(command)
		if err != nil {
			t.Fatalf("apply lifecycle command %d: %v", index+1, err)
		}
	}
	return set, start, events
}

func semanticAcquireCommand(
	t *testing.T,
	set *workingset.Set,
	name string,
	scope workingset.Scope,
	expected uint64,
) workingset.AcquireCommand {
	t.Helper()
	return workingset.AcquireCommand{
		CommandID: semanticWorkingCommandID(t, "acquire-"+name), ExpectedVersion: expected,
		Actor: taskstate.AuthorityCode,
		Request: workingset.AcquireRequest{
			ID: workingset.ItemID(name), Ref: taskstate.Ref{
				URI: "evidence://semantic/" + name, Version: "1",
				Hash: traceTestDigest(name), Relation: taskstate.RefEvidence,
			},
			Role: workingset.RoleEvidence, Retention: workingset.RetentionTask,
			Scope: scope, Priority: 10, ByteCost: 8,
			Acquisition: workingset.Acquisition{
				Provider: workingset.ProviderEvidence, OperationID: "operation-" + name,
				Reason: "Required by the semantic replay lifecycle test.",
			},
		},
	}
}

func semanticWorkingCommandID(t *testing.T, name string) workingset.CommandID {
	t.Helper()
	id, err := workingset.NewCommandID("semantic-working-lifecycle", name)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
