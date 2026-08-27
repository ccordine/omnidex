package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestDirectCodingVerificationTaskResumesAndRequiresExactFreshProof(t *testing.T) {
	coordinator, store, verification := directCodingCompletionResumeFixture(t, false)
	state, err := coordinator.BeginWorkspaceVerification()
	if err != nil || state != directCodingCompletionTaskStarted {
		t.Fatalf("initial begin state=%q err=%v", state, err)
	}
	retry := directCodingRetryCognition(coordinator, store)
	state, err = retry.BeginWorkspaceVerification()
	if err != nil || state != directCodingCompletionTaskResumed {
		t.Fatalf("retry begin state=%q err=%v", state, err)
	}
	state, err = retry.CompleteWorkspaceVerification(verification)
	if err != nil || state != directCodingCompletionTaskCompleted {
		t.Fatalf("retry completion state=%q err=%v", state, err)
	}
	state, err = retry.BeginWorkspaceVerification()
	if err != nil || state != directCodingCompletionTaskAlreadyDone {
		t.Fatalf("completed begin state=%q err=%v", state, err)
	}
	version := store.ledger.Version()
	state, err = retry.CompleteWorkspaceVerification(verification)
	if err != nil || state != directCodingCompletionTaskAlreadyDone {
		t.Fatalf("completed replay state=%q err=%v", state, err)
	}
	if store.ledger.Version() != version {
		t.Fatal("exact completed verification replay mutated the persisted ledger")
	}
	proof := taskNode(t, store.ledger.MaterializedState(), directCodingVerificationTaskNodeID).VerificationRefs
	if len(proof) != 1 ||
		proof[0].URI != "verification://job/71/workspace/"+verification.MutationOperationID ||
		proof[0].Version != "v2" || proof[0].Relation != taskstate.RefVerifies {
		t.Fatalf("workspace mutation proof=%+v", proof)
	}
	changed := verification
	changed.MutationReceiptSHA256 = strings.Repeat("f", 64)
	if _, err := retry.CompleteWorkspaceVerification(changed); err == nil ||
		!strings.Contains(err.Error(), "proof conflicts") {
		t.Fatalf("changed workspace receipt proof error=%v", err)
	}
}

func TestDirectCodingDeploymentTaskResumesAndReusesExactReceiptProof(t *testing.T) {
	coordinator, store, verification := directCodingCompletionResumeFixture(t, true)
	if _, err := coordinator.BeginWorkspaceVerification(); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteWorkspaceVerification(verification); err != nil {
		t.Fatal(err)
	}
	state, err := coordinator.BeginDeployment(verification)
	if err != nil || state != directCodingCompletionTaskStarted {
		t.Fatalf("initial deployment state=%q err=%v", state, err)
	}
	retry := directCodingRetryCognition(coordinator, store)
	state, err = retry.BeginDeployment(verification)
	if err != nil || state != directCodingCompletionTaskResumed {
		t.Fatalf("resumed deployment state=%q err=%v", state, err)
	}
	receipt := strings.Repeat("a", 64)
	state, err = retry.CompleteDeployment("operation-1", receipt)
	if err != nil || state != directCodingCompletionTaskCompleted {
		t.Fatalf("deployment completion state=%q err=%v", state, err)
	}
	state, err = retry.BeginDeployment(verification)
	if err != nil || state != directCodingCompletionTaskAlreadyDone {
		t.Fatalf("completed deployment begin state=%q err=%v", state, err)
	}
	state, err = retry.CompleteDeployment("operation-1", receipt)
	if err != nil || state != directCodingCompletionTaskAlreadyDone {
		t.Fatalf("receipt replay state=%q err=%v", state, err)
	}
	if _, err := retry.CompleteDeployment("operation-1", strings.Repeat("b", 64)); err == nil ||
		!strings.Contains(err.Error(), "proof conflicts") {
		t.Fatalf("changed receipt proof error=%v", err)
	}
}

func TestDirectCodingCompletedObjectiveReplaysWithoutRedoingTasks(t *testing.T) {
	coordinator, store, verification := directCodingCompletionResumeFixture(t, false)
	if _, err := coordinator.BeginWorkspaceVerification(); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteWorkspaceVerification(verification); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.CompleteObjective(verification); err != nil {
		t.Fatal(err)
	}
	version := store.ledger.Version()
	retry := directCodingRetryCognition(coordinator, store)
	if err := retry.CompleteObjective(verification); err != nil {
		t.Fatal(err)
	}
	if store.ledger.Version() != version {
		t.Fatal("completed objective replay mutated persisted task state")
	}
	changed := verification
	changed.MutationOperationID = "workspace_mutation_" + strings.Repeat("e", 64)
	if err := retry.CompleteObjective(changed); err == nil ||
		!strings.Contains(err.Error(), "proof conflicts") {
		t.Fatalf("changed objective workspace operation proof error=%v", err)
	}
}

func TestDirectCodingSessionRoutesPersistedDeploymentToRecoveryHook(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		complete  bool
		wantState directCodingCompletionTaskDisposition
	}{
		{name: "active", wantState: directCodingCompletionTaskResumed},
		{name: "done", complete: true, wantState: directCodingCompletionTaskAlreadyDone},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			coordinator, _, verification := directCodingCompletionResumeFixture(t, true)
			if _, err := coordinator.BeginWorkspaceVerification(); err != nil {
				t.Fatal(err)
			}
			if _, err := coordinator.CompleteWorkspaceVerification(verification); err != nil {
				t.Fatal(err)
			}
			if _, err := coordinator.BeginDeployment(verification); err != nil {
				t.Fatal(err)
			}
			receipt := strings.Repeat("c", 64)
			if testCase.complete {
				if _, err := coordinator.CompleteDeployment("operation-2", receipt); err != nil {
					t.Fatal(err)
				}
			}
			hook := &directCodingDeploymentRecoveryFixture{
				cognition: coordinator, operationID: "operation-2", receiptSHA256: receipt,
			}
			session := &directCodingSession{cognition: coordinator, deploymentRecovery: hook}
			state, err := coordinator.BeginDeployment(verification)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := session.finalizeVerifiedDeployment(verification, state)
			if err != nil {
				t.Fatal(err)
			}
			if hook.calls != 1 || hook.state != testCase.wantState ||
				outcome.OperationID != hook.operationID || outcome.ReceiptSHA256 != receipt {
				t.Fatalf("hook=%+v outcome=%+v", hook, outcome)
			}
		})
	}
}

func TestDirectCodingSessionFailsLoudlyWithoutDeploymentRecoveryHook(t *testing.T) {
	coordinator, _, verification := directCodingCompletionResumeFixture(t, true)
	if _, err := coordinator.BeginWorkspaceVerification(); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteWorkspaceVerification(verification); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.BeginDeployment(verification); err != nil {
		t.Fatal(err)
	}
	state, err := coordinator.BeginDeployment(verification)
	if err != nil {
		t.Fatal(err)
	}
	session := &directCodingSession{cognition: coordinator}
	if _, err := session.finalizeVerifiedDeployment(verification, state); err == nil ||
		!strings.Contains(err.Error(), "registered recovery hook") {
		t.Fatalf("missing recovery hook error=%v", err)
	}
}

type directCodingDeploymentRecoveryFixture struct {
	cognition     *directCodingTaskCognition
	operationID   string
	receiptSHA256 string
	state         directCodingCompletionTaskDisposition
	calls         int
}

func (fixture *directCodingDeploymentRecoveryFixture) RecoverVerifiedDeployment(
	_ directCodingVerification,
	state directCodingCompletionTaskDisposition,
) (directCodingDeploymentOutcome, error) {
	fixture.calls++
	fixture.state = state
	if _, err := fixture.cognition.CompleteDeployment(
		fixture.operationID, fixture.receiptSHA256,
	); err != nil {
		return directCodingDeploymentOutcome{}, err
	}
	return directCodingDeploymentOutcome{
		OperationID: fixture.operationID, ReceiptSHA256: fixture.receiptSHA256,
		Endpoint: directCodingObservedEndpoint{
			Scheme: "http", Host: "service.example.test", Port: 18080,
			Path: directCodingDeploymentReadinessPath,
		},
	}, nil
}

func directCodingCompletionResumeFixture(
	t *testing.T,
	deployment bool,
) (*directCodingTaskCognition, *directCodingTaskCognitionTestStore, directCodingVerification) {
	t.Helper()
	_, workload, _ := applicationTaskLifecycleFixture(t)
	store := newDirectCodingTaskCognitionStore(t)
	coordinator := &directCodingTaskCognition{
		ctx: context.Background(), store: store, authority: store.authority,
		instruction: "Build the requested service.", objectiveID: "direct-coding-objective",
		taskIDs: map[string]taskstate.NodeID{}, treeTaskIDs: map[string]taskstate.NodeID{},
		treeFiles:          map[string]assemblyline.TargetTreeTransition{},
		treeDirs:           map[string]assemblyline.TargetTreeTransition{},
		verificationTaskID: directCodingVerificationTaskNodeID,
		deploymentTaskID:   directCodingDeploymentTaskNodeID, deploymentRequired: deployment,
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
	transition := assemblyline.TargetTreeTransition{
		Kind: assemblyline.TargetTreeCreate, Path: "src/service.ts",
	}
	if err := coordinator.PlanTreeTransitions([]assemblyline.TargetTreeTransition{transition}); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.BeginTreeTransition(transition); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.CompleteTreeTransition(transition, "verified source"); err != nil {
		t.Fatal(err)
	}
	return coordinator, store, directCodingVerification{
		Passed: true, TestsPassed: true, Commands: []string{"npm test"}, EvidenceIDs: []int64{24},
		MutationOperationID:   "workspace_mutation_" + strings.Repeat("a", 64),
		MutationReceiptSHA256: strings.Repeat("b", 64),
	}
}

func directCodingRetryCognition(
	prior *directCodingTaskCognition,
	store *directCodingTaskCognitionTestStore,
) *directCodingTaskCognition {
	retry := *prior
	retry.authority.Attempt++
	retry.authority.WorkerID = "worker-retry"
	store.authority = retry.authority
	return &retry
}
