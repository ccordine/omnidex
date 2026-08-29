package worker

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func TestRecoveredPlainWorkspaceMutationCompletesPersistedCognitionWithoutReassembly(t *testing.T) {
	_, workload, _ := applicationTaskLifecycleFixture(t)
	store := newDirectCodingTaskCognitionStore(t)
	coordinator := &directCodingTaskCognition{
		ctx: context.Background(), store: store, authority: store.authority,
		instruction: "Build a typed workspace.", objectiveID: "direct-coding-objective",
		taskIDs: map[string]taskstate.NodeID{}, treeTaskIDs: map[string]taskstate.NodeID{},
		treeFiles:          map[string]assemblyline.TargetTreeTransition{},
		treeDirs:           map[string]assemblyline.TargetTreeTransition{},
		verificationTaskID: directCodingVerificationTaskNodeID,
		deploymentTaskID:   directCodingDeploymentTaskNodeID,
	}
	if err := coordinator.Bootstrap(workload); err != nil {
		t.Fatal(err)
	}
	for _, task := range workload.Tasks {
		if err := coordinator.Begin(task.ID); err != nil {
			t.Fatal(err)
		}
		if err := coordinator.CompleteTask(task.ID, map[string]string{
			"source": "accepted source", "test": "accepted test",
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

	root := t.TempDir()
	source, err := workspacefacts.Capture(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := workspacefacts.PlanMutation(
		context.Background(), source, "workload_"+strings.Repeat("a", 64),
		[]workspacefacts.DesiredFileState{{
			Path: "src/main.ts", Present: true,
			Content: []byte("export const ready = true;\n"), Mode: 0o644,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	stage, err := workspacefacts.StageMutation(context.Background(), source, plan)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })
	if _, err := stage.ApplyVerified(context.Background()); err != nil {
		t.Fatal(err)
	}
	current, err := workspacefacts.Capture(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	operationID := "workspace_mutation_" + strings.Repeat("b", 64)
	receiptSHA := strings.Repeat("c", 64)
	snapshot := &queue.WorkspaceMutationSnapshot{
		OperationID: operationID,
		Command: queue.WorkspaceMutationCommand{
			JobID: store.authority.JobID, StepID: store.authority.StepID,
			Generation: store.authority.Generation, ProjectLocation: root, Plan: plan,
		},
		Terminal: &queue.WorkspaceMutationTerminal{
			Result: queue.WorkspaceMutationResult{
				OperationID: operationID, VerificationSucceeded: true,
				CommandEvidenceIDs: []int64{31},
			},
			ReceiptSHA256: receiptSHA,
		},
	}
	commands := []testCommand{{
		Family: "go", Name: "go", Args: []string{"test", "./..."},
		Purpose: verificationTest, WorkspaceRole: workspaceVerificationPrimary,
	}}
	verification, err := directCodingVerificationFromWorkspaceMutation(snapshot, commands)
	if err != nil {
		t.Fatal(err)
	}
	retry := directCodingRetryCognition(coordinator, store)
	retry.taskIDs = map[string]taskstate.NodeID{}
	retry.treeTaskIDs = map[string]taskstate.NodeID{}
	retry.treeFiles = map[string]assemblyline.TargetTreeTransition{}
	retry.treeDirs = map[string]assemblyline.TargetTreeTransition{}
	if err := retry.restoreWorkspaceMutationCognition(); err != nil {
		t.Fatal(err)
	}
	if err := retry.CompleteRecoveredWorkspaceMutation(
		root, current, snapshot, verification,
	); err != nil {
		t.Fatal(err)
	}
	ledger := store.ledger.MaterializedState()
	if taskNode(t, ledger, retry.objectiveID).Status != taskstate.NodeDone ||
		taskNode(t, ledger, directCodingVerificationTaskNodeID).Status != taskstate.NodeDone {
		t.Fatalf("recovered cognition did not complete: %+v", ledger.Nodes)
	}
	for _, transition := range transitions {
		key, err := directCodingTreeTaskKey(transition)
		if err != nil {
			t.Fatal(err)
		}
		if taskNode(t, ledger, retry.treeTaskIDs[key]).Status != taskstate.NodeDone {
			t.Fatalf("tree transition %s is incomplete", transition.Path)
		}
	}
	if raw, err := os.ReadFile(root + "/src/main.ts"); err != nil ||
		string(raw) != "export const ready = true;\n" {
		t.Fatalf("recovery changed verified host bytes: %q error=%v", raw, err)
	}
}

func TestTerminalWorkspaceVerificationBindsOnlyPrimaryEvidenceAndReceipt(t *testing.T) {
	snapshot := &queue.WorkspaceMutationSnapshot{
		OperationID: "workspace_mutation_" + strings.Repeat("d", 64),
		Terminal: &queue.WorkspaceMutationTerminal{
			Result: queue.WorkspaceMutationResult{
				OperationID:           "workspace_mutation_" + strings.Repeat("d", 64),
				VerificationSucceeded: true, CommandEvidenceIDs: []int64{41, 42},
			},
			ReceiptSHA256: strings.Repeat("e", 64),
		},
	}
	commands := []testCommand{
		{Family: "go", Name: "go", Args: []string{"test", "./..."}, Purpose: verificationTest, WorkspaceRole: workspaceVerificationPrimary},
		{Family: "go", Name: "go", Args: []string{"clean"}, Purpose: verificationSetup, WorkspaceRole: workspaceVerificationCleanup},
	}
	verification, err := directCodingVerificationFromWorkspaceMutation(snapshot, commands)
	if err != nil {
		t.Fatal(err)
	}
	if len(verification.Commands) != 1 || len(verification.EvidenceIDs) != 1 ||
		verification.EvidenceIDs[0] != 41 ||
		verification.MutationOperationID != snapshot.OperationID ||
		verification.MutationReceiptSHA256 != snapshot.Terminal.ReceiptSHA256 {
		t.Fatalf("terminal verification=%+v", verification)
	}
}
