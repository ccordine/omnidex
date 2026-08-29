package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

func (c *directCodingTaskCognition) validateSealedAppliedWorkingSet(
	ledger *taskstate.Ledger,
	objective, verification, deployment taskstate.Node,
	children map[taskstate.NodeID]taskstate.Node,
) error {
	if ledger == nil {
		return fmt.Errorf("sealed applied recovery requires one restored task ledger")
	}
	snapshot, err := c.store.CurrentWorkingSet(c.ctx, c.authority.JobID)
	if err != nil {
		return fmt.Errorf("restore sealed applied working set: %w", err)
	}
	set, err := workingset.Restore(snapshot)
	if err != nil {
		return fmt.Errorf("validate sealed applied working set: %w", err)
	}
	owner := set.Owner()
	if owner.LedgerID != ledger.ID() || owner.JobID != c.authority.JobID ||
		owner.Generation != c.authority.Generation || set.Status() != workingset.StatusActive {
		return fmt.Errorf("sealed applied working set owner or status differs from current cognition")
	}
	if err := validateRecoveredCompletionWorkingSetItem(set, verification, c.authority.JobID); err != nil {
		return fmt.Errorf("workspace verification working set: %w", err)
	}
	if err := validateRecoveredCompletionWorkingSetItem(set, deployment, c.authority.JobID); err != nil {
		return fmt.Errorf("deployment working set: %w", err)
	}
	for _, child := range children {
		if child.ID == deployment.ID {
			continue
		}
		if _, superseded := ledger.NodeSupersession(child.ID); superseded {
			continue
		}
		scope := workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(child.ID)}
		if child.Status != taskstate.NodeDone || !set.ScopeClosed(scope) {
			return fmt.Errorf("completed pre-deployment task %q has an open working-set scope", child.ID)
		}
	}
	return validateRecoveredObjectiveWorkingSetItem(set, objective, c.authority.JobID)
}

func validateRecoveredCompletionWorkingSetItem(
	set *workingset.Set,
	node taskstate.Node,
	jobID int64,
) error {
	scope := workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(node.ID)}
	id := workingset.ItemID("completion-" + directCodingDigest(string(node.ID)))
	item, exists := set.Item(id)
	if !exists || item.ID != id || item.Role != workingset.RoleTask ||
		item.Retention != workingset.RetentionTask ||
		item.Ref != (taskstate.Ref{
			URI: fmt.Sprintf("task://job/%d/%s", jobID, node.ID), Version: "v1",
			Hash: directCodingDigest(string(node.ID)), Relation: taskstate.RefSource,
		}) || !recoveredWorkingSetAcquisition(item.Acquisition) {
		return fmt.Errorf("completion item authority is invalid")
	}
	return validateRecoveredWorkingSetScope(set, node, item, scope, workingset.RetentionTask)
}

func validateRecoveredObjectiveWorkingSetItem(
	set *workingset.Set,
	node taskstate.Node,
	jobID int64,
) error {
	scope := workingset.Scope{Kind: workingset.ScopeObjective, ID: workingset.ScopeID(node.ID)}
	item, exists := set.Item("objective")
	prefix := fmt.Sprintf("workload://job/%d/", jobID)
	if !exists || item.Role != workingset.RoleObjective ||
		item.Retention != workingset.RetentionObjective ||
		!strings.HasPrefix(item.Ref.URI, prefix) ||
		item.Ref.Hash != strings.TrimPrefix(item.Ref.URI, prefix) ||
		!directCodingRollbackDockerIDPattern.MatchString(item.Ref.Hash) ||
		item.Ref.Version != "v1" || item.Ref.Relation != taskstate.RefSource ||
		!recoveredWorkingSetAcquisition(item.Acquisition) {
		return fmt.Errorf("objective item authority is invalid")
	}
	return validateRecoveredWorkingSetScope(set, node, item, scope, workingset.RetentionObjective)
}

func validateRecoveredWorkingSetScope(
	set *workingset.Set,
	node taskstate.Node,
	item workingset.Item,
	scope workingset.Scope,
	retention workingset.Retention,
) error {
	closed := set.ScopeClosed(scope)
	if node.Status == taskstate.NodeActive && closed {
		return fmt.Errorf("active cognition has a preclosed scope")
	}
	if node.Status != taskstate.NodeActive && node.Status != taskstate.NodeDone {
		return fmt.Errorf("recovered cognition scope has nonrecoverable node status %s", node.Status)
	}
	if closed {
		if node.Status != taskstate.NodeDone || item.State != workingset.ItemReleased || len(item.Memberships) != 0 {
			return fmt.Errorf("closed cognition scope has invalid terminal item lifecycle")
		}
		return nil
	}
	if item.State != workingset.ItemResident || len(item.Memberships) != 1 ||
		item.Memberships[0] != (workingset.Membership{Scope: scope, Retention: retention}) {
		return fmt.Errorf("open cognition scope lacks exact resident membership")
	}
	return nil
}

func recoveredWorkingSetAcquisition(value workingset.Acquisition) bool {
	return value.Provider == workingset.ProviderTaskState &&
		value.OperationID == "direct-coding-task-cognition" && value.Reason != ""
}
