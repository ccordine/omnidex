package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestSealedAppliedCognitionPrevalidatesWorkingSetBeforeMutation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(*testing.T, *directCodingTaskCognitionTestStore)
		view  func(workingset.Snapshot) (workingset.Snapshot, error)
	}{
		{name: "missing", view: func(workingset.Snapshot) (workingset.Snapshot, error) {
			return workingset.Snapshot{}, queue.ErrWorkingSetNotFound
		}},
		{name: "invalid owner", view: func(snapshot workingset.Snapshot) (workingset.Snapshot, error) {
			snapshot.Owner.Generation++
			return snapshot, nil
		}},
		{name: "missing deployment item", view: func(snapshot workingset.Snapshot) (workingset.Snapshot, error) {
			id := workingset.ItemID("completion-" + directCodingDigest(string(directCodingDeploymentTaskNodeID)))
			items := snapshot.Items[:0]
			for _, item := range snapshot.Items {
				if item.ID != id {
					items = append(items, item)
				}
			}
			snapshot.Items = items
			return snapshot, nil
		}},
		{name: "active deployment scope preclosed", setup: func(t *testing.T, store *directCodingTaskCognitionTestStore) {
			if _, err := store.set.CloseScope(workingset.Scope{
				Kind: workingset.ScopeTask, ID: workingset.ScopeID(directCodingDeploymentTaskNodeID),
			}, "Invalid premature deployment close."); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "active objective scope preclosed", setup: func(t *testing.T, store *directCodingTaskCognitionTestStore) {
			if _, err := store.set.CloseScope(workingset.Scope{
				Kind: workingset.ScopeObjective, ID: "direct-coding-objective",
			}, "Invalid premature objective close."); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "verification scope open"},
		{name: "ordinary child scope open"},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			coordinator, store, _ := sealedAppliedCognitionFixture(t)
			if testCase.setup != nil {
				testCase.setup(t, store)
			}
			viewMutation := testCase.view
			if testCase.name == "verification scope open" {
				viewMutation = reopenRecoveredTaskScope(
					directCodingVerificationTaskNodeID,
					workingset.ItemID("completion-"+directCodingDigest(string(directCodingVerificationTaskNodeID))),
				)
			}
			if testCase.name == "ordinary child scope open" {
				for _, node := range store.ledger.MaterializedState().Nodes {
					if strings.HasPrefix(string(node.ID), "direct-coding-task-") {
						viewMutation = reopenRecoveredTaskScope(
							node.ID, workingset.ItemID("task-"+strings.TrimPrefix(string(node.ID), "direct-coding-task-")),
						)
						break
					}
				}
			}
			retry := directCodingRetryCognition(coordinator, store)
			view := &sealedAppliedCognitionMutationStore{base: store, workingSet: viewMutation}
			retry.store = view
			ledgerVersion, setVersion := store.ledger.Version(), store.set.Version()
			if err := retry.CompleteSealedAppliedRecovery(
				"operation-1", strings.Repeat("a", 64),
			); err == nil {
				t.Fatal("invalid sealed applied working set recovered")
			}
			if view.taskApplyCalls != 0 || view.workingSetApplyCalls != 0 ||
				store.ledger.Version() != ledgerVersion || store.set.Version() != setVersion {
				t.Fatalf("working-set prevalidation mutated state: task=%d ws=%d ledger=%d set=%d",
					view.taskApplyCalls, view.workingSetApplyCalls,
					store.ledger.Version(), store.set.Version())
			}
		})
	}
}

func reopenRecoveredTaskScope(
	nodeID taskstate.NodeID,
	itemID workingset.ItemID,
) func(workingset.Snapshot) (workingset.Snapshot, error) {
	return func(snapshot workingset.Snapshot) (workingset.Snapshot, error) {
		scope := workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(nodeID)}
		closed := snapshot.ClosedScopes[:0]
		for _, value := range snapshot.ClosedScopes {
			if value != scope {
				closed = append(closed, value)
			}
		}
		snapshot.ClosedScopes = closed
		for index := range snapshot.Items {
			item := &snapshot.Items[index]
			if item.ID == itemID {
				item.State, item.ReleasedTick, item.DispositionReason = workingset.ItemResident, 0, ""
				item.Memberships = []workingset.Membership{{Scope: scope, Retention: workingset.RetentionTask}}
			}
		}
		return snapshot, nil
	}
}
