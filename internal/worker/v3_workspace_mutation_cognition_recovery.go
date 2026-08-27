package worker

import (
	"fmt"
	"os"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func newRecoveredDirectCodingTaskCognition(
	runtime *nativeRuntimeV3,
	request directCodingRequest,
) (*directCodingTaskCognition, error) {
	if runtime == nil || runtime.svc == nil || runtime.svc.repo == nil || runtime.claim == nil {
		return nil, fmt.Errorf("restore direct-coding cognition requires one claimed runtime")
	}
	cognition := &directCodingTaskCognition{
		ctx: runtime.ctx, store: runtime.svc.repo, authority: runtime.claim.Authority,
		instruction: request.Instruction, objectiveID: taskstate.NodeID("direct-coding-objective"),
		taskIDs:            make(map[string]taskstate.NodeID),
		treeTaskIDs:        make(map[string]taskstate.NodeID),
		treeFiles:          make(map[string]assemblyline.TargetTreeTransition),
		treeDirs:           make(map[string]assemblyline.TargetTreeTransition),
		verificationTaskID: directCodingVerificationTaskNodeID,
		deploymentTaskID:   directCodingDeploymentTaskNodeID,
	}
	if err := cognition.restoreWorkspaceMutationCognition(); err != nil {
		return nil, err
	}
	return cognition, nil
}

func (c *directCodingTaskCognition) restoreWorkspaceMutationCognition() error {
	if c == nil || c.authority.JobID <= 0 || c.authority.Generation <= 0 ||
		c.authority.StepID <= 0 {
		return fmt.Errorf("restore workspace mutation cognition requires current step authority")
	}
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	root, exists := ledger.Node("goal:root")
	if !exists || root.Kind != taskstate.NodeGoal || root.Status != taskstate.NodeActive {
		return fmt.Errorf("restore workspace mutation cognition requires the active queue root goal")
	}
	objective, exists := ledger.Node(c.objectiveID)
	if !exists || objective.Kind != taskstate.NodeObjective ||
		objective.ParentID != root.ID || objective.CreatedBy != taskstate.AuthorityCode ||
		objective.CreatedStepID == nil || *objective.CreatedStepID != c.authority.StepID ||
		objective.Status != taskstate.NodeActive && objective.Status != taskstate.NodeDone {
		return fmt.Errorf("restore workspace mutation cognition found invalid objective authority")
	}
	verificationFound := false
	children := 0
	for _, node := range ledger.Nodes() {
		if node.ParentID != c.objectiveID || node.ObjectiveID != c.objectiveID {
			continue
		}
		children++
		if _, superseded := ledger.NodeSupersession(node.ID); superseded {
			return fmt.Errorf("restore workspace mutation cognition found superseded child %q", node.ID)
		}
		if !workspaceMutationCognitionHasDecomposition(ledger, c.objectiveID, node.ID) {
			return fmt.Errorf("restore workspace mutation cognition child %q lacks decomposition authority", node.ID)
		}
		switch {
		case node.ID == c.verificationTaskID:
			if !directCodingRecoveredCompletionTask(node, c.objectiveID, c.authority.StepID) {
				return fmt.Errorf("restore workspace mutation cognition has invalid verification task")
			}
			verificationFound = true
		case node.ID == c.deploymentTaskID:
			if !directCodingRecoveredCompletionTask(node, c.objectiveID, c.authority.StepID) {
				return fmt.Errorf("restore workspace mutation cognition has invalid deployment task")
			}
			c.deploymentRequired = true
		case strings.HasPrefix(string(node.ID), "direct-coding-tree-"):
			transition, err := directCodingTreeTransitionFromNode(
				node, c.objectiveID, c.authority.StepID,
			)
			if err != nil {
				return err
			}
			key, err := directCodingTreeTaskKey(transition)
			if err != nil {
				return err
			}
			if prior := c.treeTaskIDs[key]; prior != "" && prior != node.ID {
				return fmt.Errorf("restore workspace mutation cognition repeats tree transition %q", key)
			}
			c.treeTaskIDs[key] = node.ID
			if transition.Kind == assemblyline.TargetTreeEnsureDirectory {
				c.treeDirs[transition.Path] = transition
			} else {
				c.treeFiles[transition.Path] = transition
			}
		case strings.HasPrefix(string(node.ID), "direct-coding-task-"):
			if !directCodingRecoveredCompletionTask(node, c.objectiveID, c.authority.StepID) ||
				node.Status != taskstate.NodeDone ||
				!directCodingRecoveredNodeCompletedByStep(node, c.authority.StepID) {
				return fmt.Errorf("restore workspace mutation cognition source task %q is incomplete", node.ID)
			}
			c.taskIDs[string(node.ID)] = node.ID
		default:
			return fmt.Errorf("restore workspace mutation cognition found unregistered child %q", node.ID)
		}
	}
	if children == 0 || !verificationFound || len(c.taskIDs) == 0 || len(c.treeTaskIDs) == 0 {
		return fmt.Errorf("restore workspace mutation cognition is missing persisted obligations")
	}
	return nil
}

func workspaceMutationCognitionHasDecomposition(
	ledger *taskstate.Ledger,
	objectiveID, childID taskstate.NodeID,
) bool {
	if ledger == nil {
		return false
	}
	for _, edge := range ledger.Edges() {
		if edge.Kind == taskstate.EdgeDecomposes &&
			edge.From == objectiveID && edge.To == childID {
			return true
		}
	}
	return false
}

func (c *directCodingTaskCognition) CompleteRecoveredWorkspaceMutation(
	root string,
	current workspacefacts.Snapshot,
	snapshot *queue.WorkspaceMutationSnapshot,
	verification directCodingVerification,
) error {
	if c == nil || snapshot == nil || snapshot.Terminal == nil ||
		root != snapshot.Command.Plan.WorkspaceRoot ||
		verification.MutationOperationID != snapshot.OperationID ||
		verification.MutationReceiptSHA256 != snapshot.Terminal.ReceiptSHA256 {
		return fmt.Errorf("complete recovered workspace mutation requires exact terminal cognition authority")
	}
	if err := snapshot.Command.Plan.VerifyExpected(current); err != nil {
		return fmt.Errorf("complete recovered workspace mutation reality: %w", err)
	}
	if err := c.completeRecoveredTreeTasks(root, current, snapshot); err != nil {
		return err
	}
	begin, err := c.BeginWorkspaceVerification()
	if err != nil {
		return err
	}
	complete, err := c.CompleteWorkspaceVerification(verification)
	if err != nil {
		return err
	}
	if err := validateDirectCodingVerificationResume(begin, complete); err != nil {
		return err
	}
	if c.deploymentRequired {
		return fmt.Errorf(
			"recovered verified workspace requires persisted deployment continuation before objective completion",
		)
	}
	return c.CompleteObjective(verification)
}

func (c *directCodingTaskCognition) completeRecoveredTreeTasks(
	root string,
	current workspacefacts.Snapshot,
	snapshot *queue.WorkspaceMutationSnapshot,
) error {
	entries := make(map[string]workspacefacts.Entry, len(current.Entries))
	for _, entry := range current.Entries {
		entries[entry.Path] = entry
	}
	for remaining := len(c.treeTaskIDs); remaining > 0; {
		ledger, err := c.ledger()
		if err != nil {
			return err
		}
		activeID := taskstate.NodeID("")
		done := 0
		for _, id := range c.treeTaskIDs {
			node, exists := ledger.Node(id)
			if !exists {
				return fmt.Errorf("recovered direct-coding tree task %q disappeared", id)
			}
			switch node.Status {
			case taskstate.NodeDone:
				if err := validateRecoveredTreeTaskCompletion(
					node, c.authority.JobID, c.authority.StepID,
				); err != nil {
					return err
				}
				done++
			case taskstate.NodeActive:
				if activeID != "" {
					return fmt.Errorf("recovered workspace cognition has multiple active tree tasks")
				}
				activeID = id
			case taskstate.NodePending, taskstate.NodeReady:
			default:
				return fmt.Errorf("recovered direct-coding tree task %q has invalid status %q", id, node.Status)
			}
		}
		if done == len(c.treeTaskIDs) {
			return nil
		}
		var node taskstate.Node
		if activeID != "" {
			node, _ = ledger.Node(activeID)
		} else {
			var runnable bool
			node, runnable = ledger.NextRunnableNode()
			if !runnable {
				return fmt.Errorf("recovered workspace cognition has no runnable tree task")
			}
			if !strings.HasPrefix(string(node.ID), "direct-coding-tree-") {
				return fmt.Errorf("recovered workspace cognition next task %q is not a tree obligation", node.ID)
			}
		}
		transition, err := directCodingTreeTransitionFromNode(
			node, c.objectiveID, c.authority.StepID,
		)
		if err != nil {
			return err
		}
		if err := requireRecoveredTreePath(root, entries, transition); err != nil {
			return err
		}
		if node.Status == taskstate.NodeReady {
			if err := c.BeginTreeTransition(transition); err != nil {
				return err
			}
		}
		evidenceText := fmt.Sprintf(
			"workspace_mutation=%s receipt_sha256=%s path=%s",
			snapshot.OperationID, snapshot.Terminal.ReceiptSHA256, transition.Path,
		)
		if err := c.CompleteTreeTransition(transition, evidenceText); err != nil {
			return err
		}
		remaining--
	}
	return nil
}

func validateRecoveredTreeTaskCompletion(
	node taskstate.Node,
	jobID, stepID int64,
) error {
	if !directCodingRecoveredNodeCompletedByStep(node, stepID) || len(node.VerificationRefs) != 1 {
		return fmt.Errorf("recovered direct-coding tree task %q lacks exact completion", node.ID)
	}
	proof := node.VerificationRefs[0]
	prefix := fmt.Sprintf("filesystem://job/%d/", jobID)
	if !strings.HasPrefix(proof.URI, prefix) ||
		!validRepositoryVerificationSHA256(strings.TrimPrefix(proof.URI, prefix)) ||
		proof.Version != "v1" || proof.Relation != taskstate.RefVerifies ||
		!validRepositoryVerificationSHA256(proof.Hash) {
		return fmt.Errorf("recovered direct-coding tree task %q proof is invalid", node.ID)
	}
	return nil
}

func requireRecoveredTreePath(
	root string,
	entries map[string]workspacefacts.Entry,
	transition assemblyline.TargetTreeTransition,
) error {
	if transition.Kind == assemblyline.TargetTreeEnsureDirectory {
		target, err := resolveV3WorkspaceFile(root, transition.Path)
		if err != nil {
			return err
		}
		info, err := os.Lstat(target)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("recovered direct-coding directory %q is absent or invalid", transition.Path)
		}
		return nil
	}
	entry, exists := entries[transition.Path]
	if !exists || entry.Kind != workspacefacts.EntryRegular {
		return fmt.Errorf("recovered direct-coding file %q is absent or invalid", transition.Path)
	}
	return nil
}
