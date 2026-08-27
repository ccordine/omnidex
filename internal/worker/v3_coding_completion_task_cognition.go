package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const (
	directCodingVerificationTaskNodeID taskstate.NodeID = "direct-coding-workspace-verification"
	directCodingDeploymentTaskNodeID   taskstate.NodeID = "direct-coding-persistent-deployment"
)

type directCodingCompletionTaskDisposition string

const (
	directCodingCompletionTaskStarted     directCodingCompletionTaskDisposition = "started"
	directCodingCompletionTaskResumed     directCodingCompletionTaskDisposition = "resumed"
	directCodingCompletionTaskCompleted   directCodingCompletionTaskDisposition = "completed"
	directCodingCompletionTaskAlreadyDone directCodingCompletionTaskDisposition = "already_done"
)

func (c *directCodingTaskCognition) planCompletionObligations() error {
	if c.verificationTaskID == "" {
		c.verificationTaskID = directCodingVerificationTaskNodeID
	}
	if c.deploymentTaskID == "" {
		c.deploymentTaskID = directCodingDeploymentTaskNodeID
	}
	if err := c.addCompletionTask(
		c.verificationTaskID, "Verify the complete workspace",
		"Every code-selected verification command succeeds against the materialized workspace.", 30,
	); err != nil {
		return err
	}
	prerequisites := make([]taskstate.NodeID, 0, len(c.treeTaskIDs))
	for _, id := range c.treeTaskIDs {
		prerequisites = append(prerequisites, id)
	}
	sort.Slice(prerequisites, func(left, right int) bool { return prerequisites[left] < prerequisites[right] })
	for _, prerequisite := range prerequisites {
		if err := c.addCompletionDependency(c.verificationTaskID, prerequisite); err != nil {
			return err
		}
	}
	if !c.deploymentRequired {
		return nil
	}
	if err := c.addCompletionTask(
		c.deploymentTaskID, "Keep the verified service running on the current host",
		"The exact generated service is healthy at a persisted endpoint after restart verification.", 20,
	); err != nil {
		return err
	}
	return c.addCompletionDependency(c.deploymentTaskID, c.verificationTaskID)
}

func (c *directCodingTaskCognition) BeginWorkspaceVerification() (
	directCodingCompletionTaskDisposition,
	error,
) {
	return c.beginCompletionTask(c.verificationTaskID, "workspace verification")
}

func (c *directCodingTaskCognition) CompleteWorkspaceVerification(
	verification directCodingVerification,
) (directCodingCompletionTaskDisposition, error) {
	if err := verification.validate(); err != nil || !verification.Passed {
		return "", fmt.Errorf("workspace verification task requires successful real verification")
	}
	proof := directCodingVerificationProof(c.authority.JobID, verification)
	return c.completeCompletionTask(
		c.verificationTaskID, proof, "real workspace verification passed",
	)
}

func (c *directCodingTaskCognition) BeginDeployment(
	verification directCodingVerification,
) (directCodingCompletionTaskDisposition, error) {
	if !c.deploymentRequired {
		return "", fmt.Errorf("persistent deployment task was not requested")
	}
	if err := verification.validate(); err != nil || !verification.Passed {
		return "", fmt.Errorf("persistent deployment requires successful workspace verification")
	}
	return c.beginCompletionTask(c.deploymentTaskID, "persistent deployment")
}

func (c *directCodingTaskCognition) CompleteDeployment(
	sourceRef, receiptSHA256 string,
) (directCodingCompletionTaskDisposition, error) {
	if !c.deploymentRequired {
		return "", fmt.Errorf("persistent deployment task was not requested")
	}
	if strings.TrimSpace(sourceRef) == "" || len(receiptSHA256) != 64 {
		return "", fmt.Errorf("persistent deployment task requires one durable healthy receipt")
	}
	proof := taskstate.Ref{
		URI: "deployment://" + sourceRef, Version: "v1", Hash: receiptSHA256,
		Relation: taskstate.RefVerifies,
	}
	return c.completeCompletionTask(
		c.deploymentTaskID, proof, "persistent current-host deployment verified healthy",
	)
}

func (c *directCodingTaskCognition) addCompletionTask(
	id taskstate.NodeID,
	title, criterion string,
	priority int,
) error {
	stepID := c.authority.StepID
	if err := c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		if existing, exists := ledger.Node(id); exists {
			if existing.Kind != taskstate.NodeTask || !existing.InlineExecution ||
				existing.ParentID != c.objectiveID || existing.ObjectiveID != c.objectiveID {
				return nil, fmt.Errorf("incompatible persisted completion task %q", id)
			}
			return nil, nil
		}
		commandID, err := c.commandID("add-completion-task", string(id))
		if err != nil {
			return nil, err
		}
		return taskstate.AddNodeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID: id, ParentID: c.objectiveID, ObjectiveID: c.objectiveID, Kind: taskstate.NodeTask,
			InlineExecution: true, Title: title, Priority: priority, CreatedStepID: &stepID,
			AcceptanceCriteria: []string{criterion}, Metadata: taskstate.EmptyJSONObject(),
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
		commandID, err := c.commandID("completion-decomposes", string(id))
		if err != nil {
			return nil, err
		}
		return taskstate.AddEdgeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID:   taskstate.EdgeID("direct-coding-completion-decomposes-" + directCodingDigest(string(id))),
			Kind: taskstate.EdgeDecomposes, From: c.objectiveID, To: id,
		}, nil
	})
}

func (c *directCodingTaskCognition) addCompletionDependency(
	dependent, prerequisite taskstate.NodeID,
) error {
	return c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		for _, edge := range ledger.Edges() {
			if edge.Kind == taskstate.EdgeDependsOn && edge.From == dependent && edge.To == prerequisite {
				return nil, nil
			}
		}
		commandID, err := c.commandID("completion-dependency", string(dependent), string(prerequisite))
		if err != nil {
			return nil, err
		}
		return taskstate.AddEdgeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID:   taskstate.EdgeID("direct-coding-completion-dependency-" + directCodingDigest(string(dependent)+"\x00"+string(prerequisite))),
			Kind: taskstate.EdgeDependsOn, From: dependent, To: prerequisite,
		}, nil
	})
}

func (c *directCodingTaskCognition) beginCompletionTask(
	id taskstate.NodeID,
	label string,
) (directCodingCompletionTaskDisposition, error) {
	ledger, err := c.ledger()
	if err != nil {
		return "", err
	}
	node, exists := ledger.Node(id)
	if !exists {
		return "", fmt.Errorf("direct coding task cognition has no persisted %s task", label)
	}
	switch node.Status {
	case taskstate.NodeReady:
		next, runnable := ledger.NextRunnableNode()
		if !runnable || next.ID != id {
			return "", fmt.Errorf("direct coding task cognition cannot start %s before persisted prerequisites", label)
		}
		if err := c.transition(id, taskstate.NodeActive, nil, nil); err != nil {
			return "", err
		}
		if err := c.acquireCompletionTask(id); err != nil {
			return "", err
		}
		return directCodingCompletionTaskStarted, nil
	case taskstate.NodeActive:
		if err := c.acquireCompletionTask(id); err != nil {
			return "", err
		}
		return directCodingCompletionTaskResumed, nil
	case taskstate.NodeDone:
		return directCodingCompletionTaskAlreadyDone, nil
	default:
		return "", fmt.Errorf(
			"direct coding task cognition cannot resume %s from persisted status %q",
			label, node.Status,
		)
	}
}

func (c *directCodingTaskCognition) acquireCompletionTask(id taskstate.NodeID) error {
	return c.acquire(
		"completion-"+directCodingDigest(string(id)), workingset.RoleTask, workingset.RetentionTask,
		workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(id)},
		taskstate.Ref{URI: fmt.Sprintf("task://job/%d/%s", c.authority.JobID, id), Version: "v1", Hash: directCodingDigest(string(id)), Relation: taskstate.RefSource},
		len(id), "retain active code-owned completion obligation",
	)
}

func (c *directCodingTaskCognition) completeCompletionTask(
	id taskstate.NodeID,
	proof taskstate.Ref,
	reason string,
) (directCodingCompletionTaskDisposition, error) {
	ledger, err := c.ledger()
	if err != nil {
		return "", err
	}
	node, exists := ledger.Node(id)
	if !exists {
		return "", fmt.Errorf("direct coding completion task %q does not exist", id)
	}
	if node.Status == taskstate.NodeDone {
		if !directCodingCompletionProofEqual(node.VerificationRefs, proof) {
			return "", fmt.Errorf("direct coding completion task %q proof conflicts with persisted completion", id)
		}
		if err := c.closeCompletionTask(id, reason); err != nil {
			return "", err
		}
		return directCodingCompletionTaskAlreadyDone, nil
	}
	if node.Status != taskstate.NodeActive {
		return "", fmt.Errorf("direct coding completion task %q is not active", id)
	}
	stepID := c.authority.StepID
	if err := c.transition(id, taskstate.NodeDone, &stepID, []taskstate.Ref{proof}); err != nil {
		return "", err
	}
	if err := c.closeCompletionTask(id, reason); err != nil {
		return "", err
	}
	return directCodingCompletionTaskCompleted, nil
}

func (c *directCodingTaskCognition) closeCompletionTask(
	id taskstate.NodeID,
	reason string,
) error {
	if err := c.closeScope(
		workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(id)}, reason,
	); err != nil {
		return err
	}
	return c.promoteReady()
}

func directCodingCompletionProofEqual(refs []taskstate.Ref, expected taskstate.Ref) bool {
	if len(refs) != 1 {
		return false
	}
	actual := refs[0]
	return actual.URI == expected.URI && actual.Version == expected.Version &&
		actual.Hash == expected.Hash && actual.Relation == expected.Relation
}
