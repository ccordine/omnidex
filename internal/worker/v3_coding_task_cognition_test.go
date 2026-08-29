package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestDirectCodingTaskCognitionPersistsObjectiveTaskAndVerificationSequence(t *testing.T) {
	_, workload, _ := applicationTaskLifecycleFixture(t)
	store := newDirectCodingTaskCognitionStore(t)
	coordinator := &directCodingTaskCognition{
		ctx: context.Background(), store: store, authority: store.authority,
		instruction: "Build the operations console.", objectiveID: "direct-coding-objective",
		taskIDs: map[string]taskstate.NodeID{}, treeTaskIDs: map[string]taskstate.NodeID{},
		treeFiles: map[string]assemblyline.TargetTreeTransition{}, treeDirs: map[string]assemblyline.TargetTreeTransition{},
	}
	if err := coordinator.Bootstrap(workload); err != nil {
		t.Fatal(err)
	}
	ledger := store.ledger.MaterializedState()
	objective := taskNode(t, ledger, "direct-coding-objective")
	if objective.Kind != taskstate.NodeObjective || objective.Status != taskstate.NodeActive {
		t.Fatalf("objective=%+v", objective)
	}
	for _, task := range workload.Tasks {
		id := coordinator.taskIDs[task.ID]
		node := taskNode(t, ledger, id)
		if node.Kind != taskstate.NodeTask || !node.InlineExecution || node.Status != taskstate.NodeReady || node.AssignedStepID != nil {
			t.Fatalf("task %s=%+v", task.ID, node)
		}
		if !taskCognitionHasEdge(ledger, taskstate.EdgeDecomposes, "direct-coding-objective", id) {
			t.Fatalf("task %s is not explicitly decomposed from its objective", task.ID)
		}
	}
	if store.set == nil || len(store.set.Items()) != 2 {
		t.Fatalf("bootstrap working set=%+v", store.set)
	}

	for _, task := range workload.Tasks {
		if err := coordinator.Begin(task.ID); err != nil {
			t.Fatalf("begin %s: %v", task.ID, err)
		}
		if err := coordinator.CompleteTask(task.ID, map[string]string{
			"feature": "export function Feature() {}", "acceptance": "test('feature', () => {})",
		}); err != nil {
			t.Fatalf("complete %s: %v", task.ID, err)
		}
	}
	transitions := []assemblyline.TargetTreeTransition{
		{Kind: assemblyline.TargetTreeEnsureDirectory, Path: "src"},
		{Kind: assemblyline.TargetTreeEnsureDirectory, Path: "src/features"},
		{Kind: assemblyline.TargetTreeCreate, Path: "src/features/Feature.ts"},
	}
	if err := coordinator.PlanTreeTransitions(transitions); err != nil {
		t.Fatal(err)
	}
	for _, transition := range transitions {
		if err := coordinator.BeginTreeTransition(transition); err != nil {
			t.Fatalf("begin tree %s: %v", transition.Path, err)
		}
		if err := coordinator.CompleteTreeTransition(transition, "verified "+transition.Path); err != nil {
			t.Fatalf("complete tree %s: %v", transition.Path, err)
		}
	}
	verification := directCodingVerification{
		Passed: true, TestsPassed: true, Commands: []string{"npm run typecheck", "npm test"},
		EvidenceIDs:         []int64{21, 22},
		MutationOperationID: "workspace_mutation_" + strings.Repeat("a", 64), MutationReceiptSHA256: strings.Repeat("b", 64),
	}
	if _, err := coordinator.BeginWorkspaceVerification(); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteWorkspaceVerification(verification); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.CompleteObjective(verification); err != nil {
		t.Fatal(err)
	}
	ledger = store.ledger.MaterializedState()
	objective = taskNode(t, ledger, "direct-coding-objective")
	if objective.Status != taskstate.NodeDone || objective.CompletedStepID == nil || *objective.CompletedStepID != store.authority.StepID {
		t.Fatalf("completed objective=%+v", objective)
	}
	for _, task := range workload.Tasks {
		node := taskNode(t, ledger, coordinator.taskIDs[task.ID])
		if node.Status != taskstate.NodeDone || node.CompletedStepID == nil || *node.CompletedStepID != store.authority.StepID {
			t.Fatalf("completed task %s=%+v", task.ID, node)
		}
	}
	if !store.set.ScopeClosed(workingset.Scope{Kind: workingset.ScopeObjective, ID: workingset.ScopeID(coordinator.objectiveID)}) {
		t.Fatal("objective working-set scope was not closed after real verification")
	}
}

func TestDirectCodingTaskCognitionRequiresDurableDeploymentReceiptBeforeObjectiveCompletion(t *testing.T) {
	_, workload, _ := applicationTaskLifecycleFixture(t)
	store := newDirectCodingTaskCognitionStore(t)
	coordinator := &directCodingTaskCognition{
		ctx: context.Background(), store: store, authority: store.authority,
		instruction: "Build and keep the service running.", objectiveID: "direct-coding-objective",
		taskIDs: map[string]taskstate.NodeID{}, treeTaskIDs: map[string]taskstate.NodeID{},
		treeFiles: map[string]assemblyline.TargetTreeTransition{}, treeDirs: map[string]assemblyline.TargetTreeTransition{},
		verificationTaskID: directCodingVerificationTaskNodeID,
		deploymentTaskID:   directCodingDeploymentTaskNodeID, deploymentRequired: true,
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
	verification := directCodingVerification{
		Passed: true, TestsPassed: true, Commands: []string{"npm test"},
		EvidenceIDs:         []int64{23},
		MutationOperationID: "workspace_mutation_" + strings.Repeat("c", 64), MutationReceiptSHA256: strings.Repeat("d", 64),
	}
	if _, err := coordinator.BeginWorkspaceVerification(); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteWorkspaceVerification(verification); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.CompleteObjective(verification); err == nil ||
		!strings.Contains(err.Error(), "requested deployment is not complete") {
		t.Fatalf("objective completed without deployment receipt: %v", err)
	}
	if _, err := coordinator.BeginDeployment(verification); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteDeployment("operation-1", strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.CompleteObjective(verification); err != nil {
		t.Fatal(err)
	}
	if got := taskNode(t, store.ledger.MaterializedState(), coordinator.objectiveID).Status; got != taskstate.NodeDone {
		t.Fatalf("objective status=%s", got)
	}
}

func TestDirectCodingTaskCognitionQueuesAdapterBaselineAndTreeLeavesAfterSourceTasks(t *testing.T) {
	_, workload, _ := applicationTaskLifecycleFixture(t)
	store := newDirectCodingTaskCognitionStore(t)
	coordinator := &directCodingTaskCognition{
		ctx: context.Background(), store: store, authority: store.authority,
		instruction: "Build a browser workspace.", objectiveID: "direct-coding-objective",
		taskIDs: map[string]taskstate.NodeID{}, treeTaskIDs: map[string]taskstate.NodeID{},
		treeFiles: map[string]assemblyline.TargetTreeTransition{}, treeDirs: map[string]assemblyline.TargetTreeTransition{},
	}
	if err := coordinator.Bootstrap(workload); err != nil {
		t.Fatal(err)
	}
	for _, task := range workload.Tasks {
		if err := coordinator.Begin(task.ID); err != nil {
			t.Fatal(err)
		}
		if err := coordinator.CompleteTask(task.ID, map[string]string{"feature": "export {};", "acceptance": "test('feature', () => {});"}); err != nil {
			t.Fatal(err)
		}
	}
	assembly := directCodingAssembly{VersionProfileID: typeScriptBrowserVersionProfileV1, Files: []directCodingFileTask{
		{Path: "package.json", Content: "{}\n"},
		{Path: "src/main.tsx", Content: "export {};\n"},
		{Path: "src/runtime.tsx", Content: "export {};\n"},
		{Path: "src/features/Feature.tsx", Content: "export {};\n"},
		{Path: "src/features/Feature.test.tsx", Content: "export {};\n"},
		{Path: "src/App.tsx", Content: "export {};\n"},
	}}
	targetTransitions := []assemblyline.TargetTreeTransition{
		{Kind: assemblyline.TargetTreeCreate, Path: "src/features/Feature.tsx"},
		{Kind: assemblyline.TargetTreeCreate, Path: "src/features/Feature.test.tsx"},
	}
	transitions, err := directCodingAssemblyFilesystemTransitions(nil, nil, targetTransitions, assembly)
	if err != nil {
		t.Fatal(err)
	}
	if err := coordinator.PlanTreeTransitions(transitions); err != nil {
		t.Fatal(err)
	}
	for _, transition := range transitions {
		if err := coordinator.BeginTreeTransition(transition); err != nil {
			t.Fatalf("begin %s: %v", transition.Path, err)
		}
		if err := coordinator.CompleteTreeTransition(transition, "verified "+transition.Path); err != nil {
			t.Fatalf("complete %s: %v", transition.Path, err)
		}
	}
	ledger := store.ledger.MaterializedState()
	for _, path := range []string{"package.json", "src/main.tsx", "src/runtime.tsx", "src/features/Feature.tsx", "src/features/Feature.test.tsx", "src/App.tsx"} {
		found := false
		for _, node := range ledger.Nodes {
			if node.Kind == taskstate.NodeTask && node.Status == taskstate.NodeDone && node.Title != "" && (node.Title == "Create file "+path || node.Title == "Reconcile file "+path) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing completed filesystem leaf for %q", path)
		}
	}
}

func TestDirectCodingTaskCognitionAcceptsCodeOwnedDeleteLeaf(t *testing.T) {
	transition := assemblyline.TargetTreeTransition{
		Kind: assemblyline.TargetTreeDelete,
		Path: "src/obsolete.ts",
	}
	key, err := directCodingTreeTaskKey(transition)
	if err != nil {
		t.Fatal(err)
	}
	if key != string(assemblyline.TargetTreeDelete)+"\x00src/obsolete.ts" {
		t.Fatalf("key=%q", key)
	}
	title, criterion, err := directCodingTreeTaskDescription(transition)
	if err != nil {
		t.Fatal(err)
	}
	if title != "Delete file src/obsolete.ts" ||
		criterion != "The workspace does not contain file src/obsolete.ts." {
		t.Fatalf("title=%q criterion=%q", title, criterion)
	}
	if parent := directCodingTreeParentDirectory(transition); parent != "" {
		t.Fatalf("delete parent dependency=%q", parent)
	}
}

type directCodingTaskCognitionTestStore struct {
	ledger    *taskstate.Ledger
	set       *workingset.Set
	authority model.StepAttemptAuthority
}

func newDirectCodingTaskCognitionStore(t *testing.T) *directCodingTaskCognitionTestStore {
	t.Helper()
	owner := taskstate.LedgerOwner{Kind: taskstate.OwnerJob, JobID: 71, RunID: "4d36e96e-e325-11ce-bfc1-08002be10318"}
	ledgerID, err := taskstate.NewLedgerID(owner)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := taskstate.NewLedger(ledgerID, owner)
	if err != nil {
		t.Fatal(err)
	}
	store := &directCodingTaskCognitionTestStore{
		ledger: ledger, authority: model.StepAttemptAuthority{JobID: 71, Generation: 1, StepID: 9, Attempt: 1, WorkerID: "worker-test"},
	}
	store.applyTask(t, taskstate.AddNodeCommand{ID: "goal:root", Kind: taskstate.NodeGoal, Title: "Build the operations console.", Priority: 100, Metadata: taskstate.EmptyJSONObject()})
	store.applyTask(t, taskstate.PromoteReadyNodesCommand{})
	store.applyTask(t, taskstate.TransitionNodeCommand{NodeID: "goal:root", To: taskstate.NodeActive})
	return store
}

func (s *directCodingTaskCognitionTestStore) TaskLedger(context.Context, int64) (taskstate.MaterializedState, error) {
	return s.ledger.MaterializedState(), nil
}

func (s *directCodingTaskCognitionTestStore) ApplyTaskCommand(_ context.Context, jobID, generation int64, command taskstate.Command) (taskstate.Event, error) {
	if jobID != s.authority.JobID || generation != s.authority.Generation {
		return taskstate.Event{}, fmt.Errorf("unexpected task authority")
	}
	return s.ledger.Apply(command)
}

func (s *directCodingTaskCognitionTestStore) CurrentWorkingSet(context.Context, int64) (workingset.Snapshot, error) {
	if s.set == nil {
		return workingset.Snapshot{}, queue.ErrWorkingSetNotFound
	}
	return s.set.Snapshot(), nil
}

func (s *directCodingTaskCognitionTestStore) CreateCurrentWorkingSet(_ context.Context, authority model.StepAttemptAuthority, budget workingset.Budget) (workingset.Snapshot, error) {
	if authority != s.authority {
		return workingset.Snapshot{}, fmt.Errorf("unexpected working-set authority")
	}
	if s.set != nil {
		return workingset.Snapshot{}, queue.ErrWorkingSetExists
	}
	set, err := workingset.New(workingset.Owner{LedgerID: s.ledger.MaterializedState().ID, JobID: authority.JobID, Generation: authority.Generation}, budget)
	if err != nil {
		return workingset.Snapshot{}, err
	}
	s.set = set
	return set.Snapshot(), nil
}

func (s *directCodingTaskCognitionTestStore) ApplyWorkingSetCommand(_ context.Context, authority model.StepAttemptAuthority, command workingset.Command) (workingset.Event, error) {
	if authority != s.authority || s.set == nil {
		return workingset.Event{}, fmt.Errorf("unexpected working-set command")
	}
	return s.set.Apply(command)
}

func (s *directCodingTaskCognitionTestStore) applyTask(t *testing.T, command taskstate.Command) {
	t.Helper()
	prepared := taskCognitionTestCommand(t, s.ledger.Version(), command)
	if _, err := s.ledger.Apply(prepared); err != nil {
		t.Fatalf("apply %T: %v", command, err)
	}
}

func taskCognitionTestCommand(t *testing.T, version uint64, command taskstate.Command) taskstate.Command {
	t.Helper()
	id, err := taskstate.NewCommandID("direct-coding-cognition-test", fmt.Sprintf("%d", version), fmt.Sprintf("%T", command))
	if err != nil {
		t.Fatal(err)
	}
	switch typed := command.(type) {
	case taskstate.AddNodeCommand:
		typed.CommandID, typed.ExpectedVersion, typed.Actor = id, version, taskstate.AuthorityCode
		return typed
	case taskstate.PromoteReadyNodesCommand:
		typed.CommandID, typed.ExpectedVersion, typed.Actor = id, version, taskstate.AuthorityCode
		return typed
	case taskstate.TransitionNodeCommand:
		typed.CommandID, typed.ExpectedVersion, typed.Actor = id, version, taskstate.AuthorityCode
		return typed
	case taskstate.AddEntryCommand:
		typed.CommandID, typed.ExpectedVersion, typed.Actor = id, version, taskstate.AuthorityCode
		return typed
	default:
		t.Fatalf("unsupported command %T", command)
		return nil
	}
}

func taskNode(t *testing.T, state taskstate.MaterializedState, id taskstate.NodeID) taskstate.Node {
	t.Helper()
	for _, node := range state.Nodes {
		if node.ID == id {
			return node
		}
	}
	t.Fatalf("missing node %q", id)
	return taskstate.Node{}
}

func taskCognitionHasEdge(state taskstate.MaterializedState, kind taskstate.EdgeKind, from, to taskstate.NodeID) bool {
	for _, edge := range state.Edges {
		if edge.Kind == kind && edge.From == from && edge.To == to {
			return true
		}
	}
	return false
}

var _ directCodingTaskCognitionStore = (*directCodingTaskCognitionTestStore)(nil)
