package worker

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestDirectCodingTreeTaskPersistsExactStructuredTransition(t *testing.T) {
	_, workload, _ := applicationTaskLifecycleFixture(t)
	store := newDirectCodingTaskCognitionStore(t)
	coordinator := &directCodingTaskCognition{
		ctx: context.Background(), store: store, authority: store.authority,
		instruction: "Build the operations console.", objectiveID: "direct-coding-objective",
		taskIDs: map[string]taskstate.NodeID{}, treeTaskIDs: map[string]taskstate.NodeID{},
		treeFiles: map[string]assemblyline.TargetTreeTransition{},
		treeDirs:  map[string]assemblyline.TargetTreeTransition{},
	}
	if err := coordinator.Bootstrap(workload); err != nil {
		t.Fatal(err)
	}
	for _, task := range workload.Tasks {
		if err := coordinator.Begin(task.ID); err != nil {
			t.Fatal(err)
		}
		if err := coordinator.CompleteTask(task.ID, map[string]string{
			"feature": "export {};", "acceptance": "test('feature', () => {});",
		}); err != nil {
			t.Fatal(err)
		}
	}
	transitions := []assemblyline.TargetTreeTransition{
		{Kind: assemblyline.TargetTreeEnsureDirectory, Path: "src"},
		{Kind: assemblyline.TargetTreeCreate, Path: "src/main.ts"},
	}
	if err := coordinator.PlanTreeTransitions(transitions); err != nil {
		t.Fatal(err)
	}
	ledger := store.ledger.MaterializedState()
	for _, want := range transitions {
		key, err := directCodingTreeTaskKey(want)
		if err != nil {
			t.Fatal(err)
		}
		node := taskNode(t, ledger, coordinator.treeTaskIDs[key])
		got, err := directCodingTreeTransitionFromNode(node, coordinator.objectiveID, store.authority.StepID)
		if err != nil {
			t.Fatalf("restore %s: %v", want.Path, err)
		}
		if got != want {
			t.Fatalf("restored transition=%+v want=%+v", got, want)
		}
	}
}

func TestDirectCodingTreeTaskTransitionRejectsEmptyLegacyMetadata(t *testing.T) {
	node := taskstate.Node{
		ID:       taskstate.NodeID("direct-coding-tree-" + directCodingDigest("create\x00src/main.ts")),
		ParentID: "direct-coding-objective", ObjectiveID: "direct-coding-objective",
		Kind: taskstate.NodeTask, InlineExecution: true, CreatedBy: taskstate.AuthorityCode,
		CreatedStepID: func() *int64 { value := int64(9); return &value }(),
		Metadata:      taskstate.EmptyJSONObject(),
	}
	if _, err := directCodingTreeTransitionFromNode(node, "direct-coding-objective", 9); err == nil {
		t.Fatal("legacy title-only tree task was accepted as recovery authority")
	}
}
