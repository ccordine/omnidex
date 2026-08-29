package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

// directCodingTaskCognition persists the code-owned objective/task loop inside
// the one queue step that currently executes direct coding. It does not plan,
// select tools, or call a model: it records deterministic progress through the
// already accepted workload and real verification results.
type directCodingTaskCognition struct {
	ctx                context.Context
	store              directCodingTaskCognitionStore
	authority          model.StepAttemptAuthority
	instruction        string
	objectiveID        taskstate.NodeID
	taskIDs            map[string]taskstate.NodeID
	treeTaskIDs        map[string]taskstate.NodeID
	treeFiles          map[string]assemblyline.TargetTreeTransition
	treeDirs           map[string]assemblyline.TargetTreeTransition
	verificationTaskID taskstate.NodeID
	deploymentTaskID   taskstate.NodeID
	deploymentRequired bool
}

type directCodingTaskCognitionStore interface {
	TaskLedger(context.Context, int64) (taskstate.MaterializedState, error)
	ApplyTaskCommand(context.Context, int64, int64, taskstate.Command) (taskstate.Event, error)
	CurrentWorkingSet(context.Context, int64) (workingset.Snapshot, error)
	CreateCurrentWorkingSet(context.Context, model.StepAttemptAuthority, workingset.Budget) (workingset.Snapshot, error)
	ApplyWorkingSetCommand(context.Context, model.StepAttemptAuthority, workingset.Command) (workingset.Event, error)
}

const directCodingTaskCognitionSchema = "omnidex.direct-coding-task-cognition.v1"

func newDirectCodingTaskCognition(session *directCodingSession) (*directCodingTaskCognition, error) {
	if session == nil || session.runtime == nil || session.runtime.svc == nil || session.runtime.svc.repo == nil || session.runtime.claim == nil {
		return nil, fmt.Errorf("direct coding task cognition requires one claimed queue runtime")
	}
	if session.runtime.claim.Authority.JobID <= 0 || session.runtime.claim.Authority.Generation <= 0 ||
		session.runtime.claim.Authority.StepID <= 0 {
		return nil, fmt.Errorf("direct coding task cognition requires complete step authority")
	}
	deploymentRequired := session.deploymentDisposition ==
		assemblyline.ApplicationServiceDeploymentPersistCurrentHost
	if session.deploymentDisposition != assemblyline.ApplicationServiceDeploymentVerifyOnly &&
		!deploymentRequired {
		return nil, fmt.Errorf("direct coding task cognition requires one resolved deployment disposition")
	}
	return &directCodingTaskCognition{
		ctx:                session.runtime.ctx,
		store:              session.runtime.svc.repo,
		authority:          session.runtime.claim.Authority,
		instruction:        session.request.Instruction,
		objectiveID:        taskstate.NodeID("direct-coding-objective"),
		taskIDs:            make(map[string]taskstate.NodeID),
		treeTaskIDs:        make(map[string]taskstate.NodeID),
		treeFiles:          make(map[string]assemblyline.TargetTreeTransition),
		treeDirs:           make(map[string]assemblyline.TargetTreeTransition),
		verificationTaskID: directCodingVerificationTaskNodeID,
		deploymentTaskID:   directCodingDeploymentTaskNodeID,
		deploymentRequired: deploymentRequired,
	}, nil
}

func (c *directCodingTaskCognition) Bootstrap(workload assemblyline.FrozenApplicationWorkload) error {
	if c == nil {
		return fmt.Errorf("direct coding task cognition is required")
	}
	if strings.TrimSpace(workload.SHA256) == "" || strings.TrimSpace(workload.ProductQuote) == "" || len(workload.Tasks) == 0 {
		return fmt.Errorf("direct coding task cognition requires one frozen workload")
	}
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	root, ok := ledger.Node(taskstate.NodeID("goal:root"))
	if !ok || root.Kind != taskstate.NodeGoal || root.Status != taskstate.NodeActive {
		return fmt.Errorf("direct coding task cognition requires the active queue-owned root goal")
	}
	if existing, exists := ledger.Node(c.objectiveID); exists {
		if existing.Kind != taskstate.NodeObjective || existing.ParentID != root.ID || existing.Status == taskstate.NodeFailed || existing.Status == taskstate.NodeCanceled {
			return fmt.Errorf("direct coding task cognition found incompatible persisted objective %q", existing.ID)
		}
	} else if err := c.addObjective(workload); err != nil {
		return err
	}
	for _, task := range workload.Tasks {
		id, err := c.taskNodeID(task.ID)
		if err != nil {
			return err
		}
		c.taskIDs[task.ID] = id
		ledger, err = c.ledger()
		if err != nil {
			return err
		}
		if existing, exists := ledger.Node(id); exists {
			if existing.Kind != taskstate.NodeTask || !existing.InlineExecution || existing.ParentID != c.objectiveID || existing.ObjectiveID != c.objectiveID {
				return fmt.Errorf("direct coding task cognition found incompatible persisted task %q", id)
			}
			continue
		}
		if err := c.addTask(task, id); err != nil {
			return err
		}
	}
	if err := c.promoteReady(); err != nil {
		return err
	}
	ledger, err = c.ledger()
	if err != nil {
		return err
	}
	objective, ok := ledger.Node(c.objectiveID)
	if !ok {
		return fmt.Errorf("direct coding task cognition objective disappeared")
	}
	if objective.Status == taskstate.NodeReady {
		if err := c.transition(c.objectiveID, taskstate.NodeActive, nil, nil); err != nil {
			return err
		}
	} else if objective.Status != taskstate.NodeActive && objective.Status != taskstate.NodeDone {
		return fmt.Errorf("direct coding task cognition objective %q is %q", objective.ID, objective.Status)
	}
	if err := c.ensureWorkingSet(); err != nil {
		return err
	}
	return c.acquireUserAndObjective(workload)
}

func (c *directCodingTaskCognition) Begin(taskID string) error {
	id, ok := c.taskIDs[taskID]
	if !ok {
		return fmt.Errorf("direct coding task cognition has no task %q", taskID)
	}
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	next, runnable := ledger.NextRunnableNode()
	if !runnable || next.ID != id {
		return fmt.Errorf("direct coding task cognition cannot start %q before its persisted prerequisites", taskID)
	}
	if err := c.transition(id, taskstate.NodeActive, nil, nil); err != nil {
		return err
	}
	return c.acquireTask(taskID, id)
}

func (c *directCodingTaskCognition) CompleteTask(taskID string, generated map[string]string) error {
	id, ok := c.taskIDs[taskID]
	if !ok {
		return fmt.Errorf("direct coding task cognition has no task %q", taskID)
	}
	if len(generated) == 0 {
		return fmt.Errorf("direct coding task cognition task %q has no generated artifact evidence", taskID)
	}
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	node, ok := ledger.Node(id)
	if !ok || node.Status != taskstate.NodeActive {
		return fmt.Errorf("direct coding task cognition task %q is not active", taskID)
	}
	proof := directCodingTaskProof(c.authority.JobID, taskID, generated)
	stepID := c.authority.StepID
	if err := c.transition(id, taskstate.NodeDone, &stepID, []taskstate.Ref{proof}); err != nil {
		return err
	}
	if err := c.closeScope(workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(id)}, "bounded task source passed code-owned block validation"); err != nil {
		return err
	}
	return c.promoteReady()
}

func (c *directCodingTaskCognition) CompleteObjective(verification directCodingVerification) error {
	if c == nil {
		return fmt.Errorf("direct coding task cognition requires successful real verification")
	}
	if err := verification.validate(); err != nil || !verification.Passed {
		return fmt.Errorf("direct coding task cognition requires successful real verification")
	}
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	objective, ok := ledger.Node(c.objectiveID)
	if !ok || (objective.Status != taskstate.NodeActive && objective.Status != taskstate.NodeDone) {
		return fmt.Errorf("direct coding task cognition objective is neither active nor done")
	}
	for taskID, id := range c.taskIDs {
		node, exists := ledger.Node(id)
		if !exists || node.Status != taskstate.NodeDone {
			return fmt.Errorf("direct coding task cognition task %q is not complete", taskID)
		}
	}
	for leafKey, id := range c.treeTaskIDs {
		node, exists := ledger.Node(id)
		if !exists || node.Status != taskstate.NodeDone {
			return fmt.Errorf("direct coding task cognition tree leaf %q is not complete", leafKey)
		}
	}
	verificationNode, exists := ledger.Node(c.verificationTaskID)
	if !exists || verificationNode.Status != taskstate.NodeDone {
		return fmt.Errorf("direct coding task cognition workspace verification is not complete")
	}
	if c.deploymentRequired {
		deploymentNode, exists := ledger.Node(c.deploymentTaskID)
		if !exists || deploymentNode.Status != taskstate.NodeDone {
			return fmt.Errorf("direct coding task cognition requested deployment is not complete")
		}
	}
	stepID := c.authority.StepID
	proof := directCodingVerificationProof(c.authority.JobID, verification)
	if objective.Status == taskstate.NodeDone {
		if !directCodingCompletionProofEqual(objective.VerificationRefs, proof) {
			return fmt.Errorf("direct coding task cognition objective proof conflicts with persisted completion")
		}
		return c.closeScope(
			workingset.Scope{Kind: workingset.ScopeObjective, ID: workingset.ScopeID(c.objectiveID)},
			"real workspace verification passed",
		)
	}
	if err := c.transition(c.objectiveID, taskstate.NodeDone, &stepID, []taskstate.Ref{proof}); err != nil {
		return err
	}
	return c.closeScope(workingset.Scope{Kind: workingset.ScopeObjective, ID: workingset.ScopeID(c.objectiveID)}, "real workspace verification passed")
}

func (c *directCodingTaskCognition) addObjective(workload assemblyline.FrozenApplicationWorkload) error {
	stepID := c.authority.StepID
	criterion := "The complete generated workspace passes code-selected verification."
	if c.deploymentRequired {
		criterion = "The complete generated workspace passes code-selected verification and remains healthy at its persisted current-host endpoint."
	}
	return c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		commandID, err := c.commandID("add-objective", workload.SHA256)
		if err != nil {
			return nil, err
		}
		return taskstate.AddNodeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID: c.objectiveID, ParentID: taskstate.NodeID("goal:root"), Kind: taskstate.NodeObjective,
			Title: workload.ProductQuote, Priority: 100, CreatedStepID: &stepID,
			AcceptanceCriteria: []string{criterion},
			Metadata:           taskstate.EmptyJSONObject(),
		}, nil
	})
}

func (c *directCodingTaskCognition) addTask(task assemblyline.FrozenApplicationTask, id taskstate.NodeID) error {
	stepID := c.authority.StepID
	if err := c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		commandID, err := c.commandID("add-task", task.ID)
		if err != nil {
			return nil, err
		}
		return taskstate.AddNodeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID: id, ParentID: c.objectiveID, ObjectiveID: c.objectiveID, Kind: taskstate.NodeTask,
			InlineExecution: true, Title: task.RequirementQuote, Priority: 50, CreatedStepID: &stepID,
			AcceptanceCriteria: []string{task.RequirementQuote}, Metadata: taskstate.EmptyJSONObject(),
		}, nil
	}); err != nil {
		return err
	}
	return c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		for _, edge := range ledger.Edges() {
			if edge.Kind == taskstate.EdgeDecomposes && edge.From == c.objectiveID && edge.To == id {
				return nil, nil
			}
		}
		commandID, err := c.commandID("decomposes", task.ID)
		if err != nil {
			return nil, err
		}
		return taskstate.AddEdgeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID:   taskstate.EdgeID("direct-coding-decomposes-" + task.ID),
			Kind: taskstate.EdgeDecomposes, From: c.objectiveID, To: id,
		}, nil
	})
}

func (c *directCodingTaskCognition) promoteReady() error {
	return c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		if !ledger.HasPromotableNode() {
			return nil, nil
		}
		commandID, err := c.commandID("promote-ready", strconv.FormatUint(ledger.Version(), 10))
		if err != nil {
			return nil, err
		}
		return taskstate.PromoteReadyNodesCommand{CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode}, nil
	})
}

func (c *directCodingTaskCognition) transition(id taskstate.NodeID, to taskstate.NodeStatus, completedStepID *int64, refs []taskstate.Ref) error {
	return c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		commandID, err := c.commandID("transition", string(id), string(to), strconv.FormatUint(ledger.Version(), 10))
		if err != nil {
			return nil, err
		}
		return taskstate.TransitionNodeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			NodeID: id, To: to, CompletedStepID: completedStepID, VerificationRefs: refs,
		}, nil
	})
}

func (c *directCodingTaskCognition) apply(build func(*taskstate.Ledger) (taskstate.Command, error)) error {
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	command, err := build(ledger)
	if err != nil {
		return err
	}
	if command == nil {
		return nil
	}
	if _, err := c.store.ApplyTaskCommand(c.ctx, c.authority.JobID, c.authority.Generation, command); err != nil {
		return fmt.Errorf("persist direct coding task cognition: %w", err)
	}
	return nil
}

func (c *directCodingTaskCognition) ledger() (*taskstate.Ledger, error) {
	state, err := c.store.TaskLedger(c.ctx, c.authority.JobID)
	if err != nil {
		return nil, fmt.Errorf("restore direct coding task cognition ledger: %w", err)
	}
	ledger, err := taskstate.RestoreLedger(state)
	if err != nil {
		return nil, fmt.Errorf("validate direct coding task cognition ledger: %w", err)
	}
	return ledger, nil
}

func (c *directCodingTaskCognition) ensureWorkingSet() error {
	if _, err := c.store.CurrentWorkingSet(c.ctx, c.authority.JobID); err == nil {
		return nil
	} else if !errors.Is(err, queue.ErrWorkingSetNotFound) {
		return fmt.Errorf("restore direct coding working set: %w", err)
	}
	_, err := c.store.CreateCurrentWorkingSet(c.ctx, c.authority, workingset.Budget{
		MaxItems: 64, MaxBytes: 64 * 1024, MaxPinnedItems: 4, MaxPinnedBytes: 8 * 1024,
	})
	if err != nil && !errors.Is(err, queue.ErrWorkingSetExists) {
		return fmt.Errorf("create direct coding working set: %w", err)
	}
	return nil
}

func (c *directCodingTaskCognition) acquireUserAndObjective(workload assemblyline.FrozenApplicationWorkload) error {
	if err := c.acquire("user-authority", workingset.RoleUserAuthority, workingset.RetentionPinned,
		workingset.Scope{Kind: workingset.ScopeJob, ID: workingset.ScopeID("job-" + strconv.FormatInt(c.authority.JobID, 10))},
		taskstate.Ref{URI: fmt.Sprintf("task://job/%d/instruction", c.authority.JobID), Version: "v1", Hash: directCodingDigest(c.instruction), Relation: taskstate.RefSource},
		len(c.instruction), "retain immutable user authority"); err != nil {
		return err
	}
	return c.acquire("objective", workingset.RoleObjective, workingset.RetentionObjective,
		workingset.Scope{Kind: workingset.ScopeObjective, ID: workingset.ScopeID(c.objectiveID)},
		taskstate.Ref{URI: fmt.Sprintf("workload://job/%d/%s", c.authority.JobID, workload.SHA256), Version: "v1", Hash: workload.SHA256, Relation: taskstate.RefSource},
		len(workload.ProductQuote), "retain accepted objective authority")
}

func (c *directCodingTaskCognition) acquireTask(taskID string, id taskstate.NodeID) error {
	return c.acquire("task-"+taskID, workingset.RoleTask, workingset.RetentionTask,
		workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(id)},
		taskstate.Ref{URI: fmt.Sprintf("task://job/%d/%s", c.authority.JobID, taskID), Version: "v1", Hash: directCodingDigest(taskID), Relation: taskstate.RefSource},
		len(taskID), "retain active bounded task identity")
}

func (c *directCodingTaskCognition) acquire(id string, role workingset.Role, retention workingset.Retention, scope workingset.Scope, ref taskstate.Ref, byteCost int, reason string) error {
	snapshot, err := c.store.CurrentWorkingSet(c.ctx, c.authority.JobID)
	if err != nil {
		return err
	}
	for _, item := range snapshot.Items {
		if string(item.ID) == id {
			return nil
		}
	}
	commandID, err := workingset.NewCommandID(directCodingTaskCognitionSchema, strconv.FormatInt(c.authority.JobID, 10), "acquire", id)
	if err != nil {
		return err
	}
	if byteCost < 1 {
		byteCost = 1
	}
	_, err = c.store.ApplyWorkingSetCommand(c.ctx, c.authority, workingset.AcquireCommand{
		CommandID: commandID, ExpectedVersion: snapshot.Version, Actor: taskstate.AuthorityCode,
		Request: workingset.AcquireRequest{
			ID: workingset.ItemID(id), Ref: ref, Role: role, Retention: retention, Scope: scope,
			Priority: 100, ByteCost: byteCost,
			Acquisition: workingset.Acquisition{Provider: workingset.ProviderTaskState, OperationID: "direct-coding-task-cognition", Reason: reason},
		},
	})
	if err != nil {
		return fmt.Errorf("acquire direct coding working-set item %q: %w", id, err)
	}
	return nil
}

func (c *directCodingTaskCognition) closeScope(scope workingset.Scope, reason string) error {
	snapshot, err := c.store.CurrentWorkingSet(c.ctx, c.authority.JobID)
	if err != nil {
		return err
	}
	for _, closed := range snapshot.ClosedScopes {
		if closed == scope {
			return nil
		}
	}
	commandID, err := workingset.NewCommandID(directCodingTaskCognitionSchema, strconv.FormatInt(c.authority.JobID, 10), "close-scope", string(scope.Kind), string(scope.ID))
	if err != nil {
		return err
	}
	_, err = c.store.ApplyWorkingSetCommand(c.ctx, c.authority, workingset.CloseScopeCommand{
		CommandID: commandID, ExpectedVersion: snapshot.Version, Actor: taskstate.AuthorityCode, Scope: scope, Reason: reason,
	})
	if err != nil {
		return fmt.Errorf("close direct coding working-set scope %s/%s: %w", scope.Kind, scope.ID, err)
	}
	return nil
}

func (c *directCodingTaskCognition) taskNodeID(taskID string) (taskstate.NodeID, error) {
	if strings.TrimSpace(taskID) == "" || taskID != strings.TrimSpace(taskID) {
		return "", fmt.Errorf("direct coding task cognition task ID is invalid")
	}
	return taskstate.NodeID("direct-coding-task-" + taskID), nil
}

func (c *directCodingTaskCognition) commandID(parts ...string) (taskstate.CommandID, error) {
	return taskstate.NewCommandID(append([]string{directCodingTaskCognitionSchema, strconv.FormatInt(c.authority.JobID, 10), strconv.FormatInt(c.authority.Generation, 10)}, parts...)...)
}

func directCodingTaskProof(jobID int64, taskID string, generated map[string]string) taskstate.Ref {
	parts := make([]string, 0, len(generated))
	for id, source := range generated {
		parts = append(parts, id+"\x00"+source)
	}
	sort.Strings(parts)
	return taskstate.Ref{URI: fmt.Sprintf("artifact://job/%d/task/%s/generated", jobID, taskID), Version: "v1", Hash: directCodingDigest(strings.Join(parts, "\n")), Relation: taskstate.RefVerifies}
}

func directCodingVerificationProof(jobID int64, verification directCodingVerification) taskstate.Ref {
	parts := make([]string, 0, len(verification.Commands)+1)
	parts = append(parts, verification.MutationReceiptSHA256)
	for index, command := range verification.Commands {
		parts = append(parts, command+"\x00"+strconv.FormatInt(verification.EvidenceIDs[index], 10))
	}
	return taskstate.Ref{
		URI: fmt.Sprintf(
			"verification://job/%d/workspace/%s", jobID, verification.MutationOperationID,
		),
		Version: "v2", Hash: directCodingDigest(strings.Join(parts, "\n")),
		Relation: taskstate.RefVerifies,
	}
}

func validDirectCodingVerificationProof(jobID int64, proof taskstate.Ref) bool {
	prefix := fmt.Sprintf("verification://job/%d/workspace/", jobID)
	operationID := strings.TrimPrefix(proof.URI, prefix)
	return proof.URI != operationID &&
		validRepositoryVerificationOpaqueID(operationID, "workspace_mutation_") &&
		proof.Version == "v2" &&
		validRepositoryVerificationSHA256(proof.Hash) &&
		proof.Relation == taskstate.RefVerifies
}

func directCodingDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
