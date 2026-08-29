package worker

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestSealedAppliedCognitionRecoveryClosesEveryCrashWindow(t *testing.T) {
	t.Parallel()
	for _, stage := range []string{
		"deployment_active", "deployment_done_scope_open", "deployment_closed_objective_active",
		"objective_done_scope_open", "fully_closed",
	} {
		stage := stage
		t.Run(stage, func(t *testing.T) {
			t.Parallel()
			coordinator, store, verification := sealedAppliedCognitionFixture(t)
			receipt := strings.Repeat("a", 64)
			stepID := coordinator.authority.StepID
			deploymentProof := taskstate.Ref{
				URI: "deployment://operation-1", Version: "v1", Hash: receipt,
				Relation: taskstate.RefVerifies,
			}
			if stage == "deployment_done_scope_open" {
				if err := coordinator.transition(
					directCodingDeploymentTaskNodeID, taskstate.NodeDone, &stepID, []taskstate.Ref{deploymentProof},
				); err != nil {
					t.Fatal(err)
				}
			}
			if stage == "deployment_closed_objective_active" || stage == "objective_done_scope_open" {
				if _, err := coordinator.CompleteDeployment("operation-1", receipt); err != nil {
					t.Fatal(err)
				}
			}
			if stage == "objective_done_scope_open" {
				proof := directCodingVerificationProof(coordinator.authority.JobID, verification)
				if err := coordinator.transition(
					coordinator.objectiveID, taskstate.NodeDone, &stepID, []taskstate.Ref{proof},
				); err != nil {
					t.Fatal(err)
				}
			}
			if stage == "fully_closed" {
				if _, err := coordinator.CompleteDeployment("operation-1", receipt); err != nil {
					t.Fatal(err)
				}
				if err := coordinator.CompleteObjective(verification); err != nil {
					t.Fatal(err)
				}
			}
			retry := directCodingRetryCognition(coordinator, store)
			if err := retry.CompleteSealedAppliedRecovery("operation-1", receipt); err != nil {
				t.Fatal(err)
			}
			state := store.ledger.MaterializedState()
			if taskNode(t, state, directCodingDeploymentTaskNodeID).Status != taskstate.NodeDone ||
				taskNode(t, state, retry.objectiveID).Status != taskstate.NodeDone {
				t.Fatalf("recovered cognition is incomplete: %+v", state.Nodes)
			}
			for _, scope := range []workingset.Scope{
				{Kind: workingset.ScopeTask, ID: workingset.ScopeID(directCodingDeploymentTaskNodeID)},
				{Kind: workingset.ScopeObjective, ID: workingset.ScopeID(retry.objectiveID)},
			} {
				if !store.set.ScopeClosed(scope) {
					t.Fatalf("recovery left scope open: %+v", scope)
				}
			}
			version := store.ledger.Version()
			setVersion := store.set.Snapshot().Version
			if err := retry.CompleteSealedAppliedRecovery("operation-1", receipt); err != nil {
				t.Fatal(err)
			}
			if store.ledger.Version() != version || store.set.Snapshot().Version != setVersion {
				t.Fatal("sealed applied cognition replay mutated persisted state")
			}
		})
	}
}

func TestSealedAppliedCognitionRecoveryPrevalidatesBeforeMutation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		complete  bool
		objective bool
		mutate    func(*taskstate.MaterializedState)
	}{
		{name: "invalid verification proof", mutate: func(state *taskstate.MaterializedState) {
			node := recoveryStateNode(t, state, directCodingVerificationTaskNodeID)
			node.VerificationRefs[0].URI = "verification://wrong"
		}},
		{name: "incomplete child with empty objective", mutate: func(state *taskstate.MaterializedState) {
			stepID := int64(9)
			state.Nodes = append(state.Nodes, taskstate.Node{
				ID: "direct-coding-unbound-child", ParentID: "direct-coding-objective",
				Kind: taskstate.NodeTask, InlineExecution: true, Title: "Unfinished child",
				Status: taskstate.NodeActive, Priority: 50, CreatedBy: taskstate.AuthorityCode,
				CreatedStepID: &stepID, AcceptanceCriteria: []string{"must finish"},
				VerificationRefs: []taskstate.Ref{},
				Metadata:         taskstate.EmptyJSONObject(), CreatedVersion: 1, UpdatedVersion: state.Version,
			})
		}},
		{name: "conflicting deployment proof", complete: true, mutate: func(state *taskstate.MaterializedState) {
			node := recoveryStateNode(t, state, directCodingDeploymentTaskNodeID)
			node.VerificationRefs[0].Hash = strings.Repeat("b", 64)
		}},
		{name: "conflicting objective proof", complete: true, objective: true, mutate: func(state *taskstate.MaterializedState) {
			node := recoveryStateNode(t, state, "direct-coding-objective")
			node.VerificationRefs[0].Hash = strings.Repeat("c", 64)
		}},
		{name: "objective complete before deployment", complete: true, objective: true, mutate: func(state *taskstate.MaterializedState) {
			node := recoveryStateNode(t, state, directCodingDeploymentTaskNodeID)
			node.Status = taskstate.NodeActive
			node.CompletedStepID = nil
			node.VerificationRefs = []taskstate.Ref{}
		}},
		{name: "wrong deployment creation step", mutate: func(state *taskstate.MaterializedState) {
			node := recoveryStateNode(t, state, directCodingDeploymentTaskNodeID)
			wrong := int64(10)
			node.CreatedStepID = &wrong
		}},
		{name: "wrong ordinary child creation step", mutate: func(state *taskstate.MaterializedState) {
			for index := range state.Nodes {
				node := &state.Nodes[index]
				if node.Kind == taskstate.NodeTask && node.ID != directCodingVerificationTaskNodeID &&
					node.ID != directCodingDeploymentTaskNodeID {
					wrong := int64(10)
					node.CreatedStepID = &wrong
					return
				}
			}
		}},
		{name: "superseded key generation", mutate: supersedeRecoveredCognitionGeneration},
	}
	for _, testCase := range tests {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			coordinator, store, verification := sealedAppliedCognitionFixture(t)
			receipt := strings.Repeat("a", 64)
			if testCase.complete {
				if _, err := coordinator.CompleteDeployment("operation-1", receipt); err != nil {
					t.Fatal(err)
				}
			}
			if testCase.objective {
				if err := coordinator.CompleteObjective(verification); err != nil {
					t.Fatal(err)
				}
			}
			retry := directCodingRetryCognition(coordinator, store)
			view := &sealedAppliedCognitionMutationStore{base: store, mutate: testCase.mutate}
			retry.store = view
			version := store.ledger.Version()
			if err := retry.CompleteSealedAppliedRecovery("operation-1", receipt); err == nil {
				t.Fatal("invalid sealed cognition recovered")
			}
			if view.taskApplyCalls != 0 || view.workingSetApplyCalls != 0 || store.ledger.Version() != version {
				t.Fatalf("prevalidation mutated cognition: task=%d working_set=%d version=%d",
					view.taskApplyCalls, view.workingSetApplyCalls, store.ledger.Version())
			}
		})
	}
}

func sealedAppliedCognitionFixture(
	t *testing.T,
) (*directCodingTaskCognition, *directCodingTaskCognitionTestStore, directCodingVerification) {
	t.Helper()
	coordinator, store, verification := directCodingCompletionResumeFixture(t, true)
	if _, err := coordinator.BeginWorkspaceVerification(); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.CompleteWorkspaceVerification(verification); err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.BeginDeployment(verification); err != nil {
		t.Fatal(err)
	}
	return coordinator, store, verification
}

func recoveryStateNode(
	t *testing.T,
	state *taskstate.MaterializedState,
	id taskstate.NodeID,
) *taskstate.Node {
	t.Helper()
	for index := range state.Nodes {
		if state.Nodes[index].ID == id {
			return &state.Nodes[index]
		}
	}
	t.Fatalf("missing state node %q", id)
	return nil
}

type sealedAppliedCognitionMutationStore struct {
	base                 *directCodingTaskCognitionTestStore
	mutate               func(*taskstate.MaterializedState)
	workingSet           func(workingset.Snapshot) (workingset.Snapshot, error)
	taskApplyCalls       int
	workingSetApplyCalls int
}

func (store *sealedAppliedCognitionMutationStore) TaskLedger(ctx context.Context, jobID int64) (taskstate.MaterializedState, error) {
	state, err := store.base.TaskLedger(ctx, jobID)
	if err == nil && store.mutate != nil {
		store.mutate(&state)
	}
	return state, err
}

func (store *sealedAppliedCognitionMutationStore) ApplyTaskCommand(ctx context.Context, jobID, generation int64, command taskstate.Command) (taskstate.Event, error) {
	store.taskApplyCalls++
	return store.base.ApplyTaskCommand(ctx, jobID, generation, command)
}

func (store *sealedAppliedCognitionMutationStore) CurrentWorkingSet(ctx context.Context, jobID int64) (workingset.Snapshot, error) {
	snapshot, err := store.base.CurrentWorkingSet(ctx, jobID)
	if err == nil && store.workingSet != nil {
		return store.workingSet(snapshot)
	}
	return snapshot, err
}

func (store *sealedAppliedCognitionMutationStore) CreateCurrentWorkingSet(ctx context.Context, authority model.StepAttemptAuthority, budget workingset.Budget) (workingset.Snapshot, error) {
	return store.base.CreateCurrentWorkingSet(ctx, authority, budget)
}

func (store *sealedAppliedCognitionMutationStore) ApplyWorkingSetCommand(ctx context.Context, authority model.StepAttemptAuthority, command workingset.Command) (workingset.Event, error) {
	store.workingSetApplyCalls++
	return store.base.ApplyWorkingSetCommand(ctx, authority, command)
}

func supersedeRecoveredCognitionGeneration(state *taskstate.MaterializedState) {
	state.Version++
	reason := "A later generation replaced this obligation."
	ids := make([]taskstate.NodeID, 0, len(state.Nodes)-1)
	for index := range state.Nodes {
		node := &state.Nodes[index]
		if node.Kind == taskstate.NodeGoal {
			continue
		}
		ids = append(ids, node.ID)
		if node.Status != taskstate.NodeDone && node.Status != taskstate.NodeFailed && node.Status != taskstate.NodeCanceled {
			node.Status, node.StatusReason, node.UpdatedVersion = taskstate.NodeCanceled, reason, state.Version
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	state.NodeSupersessions = make([]taskstate.NodeGenerationSupersession, len(ids))
	for index, id := range ids {
		state.NodeSupersessions[index] = taskstate.NodeGenerationSupersession{
			NodeID: id, RetiringGeneration: 1, SupersededAtGeneration: 2,
			Reason: reason, CreatedVersion: state.Version,
		}
	}
}
