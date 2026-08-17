package worker

import (
	"context"
	"fmt"
	"testing"

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
		taskIDs: map[string]taskstate.NodeID{},
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
	if err := coordinator.CompleteObjective(directCodingVerification{
		Passed: true, TestsPassed: true, Commands: []string{"npm run typecheck", "npm test"},
	}); err != nil {
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

func TestDirectCodingTaskCognitionWillNotStartTaskBeforePersistedDependency(t *testing.T) {
	_, workload, _ := applicationTaskLifecycleFixture(t)
	workload.Tasks[1].DependsOn = []string{workload.Tasks[0].ID}
	store := newDirectCodingTaskCognitionStore(t)
	coordinator := &directCodingTaskCognition{
		ctx: context.Background(), store: store, authority: store.authority,
		instruction: "Build the operations console.", objectiveID: "direct-coding-objective",
		taskIDs: map[string]taskstate.NodeID{},
	}
	if err := coordinator.Bootstrap(workload); err != nil {
		t.Fatal(err)
	}
	if err := coordinator.Begin(workload.Tasks[1].ID); err == nil {
		t.Fatal("dependent task began before its persisted prerequisite")
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
