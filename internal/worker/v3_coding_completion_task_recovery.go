package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

// CompleteSealedAppliedRecovery closes only cognition that was already proven
// by the immutable workspace-verification and deployment receipts. It does not
// reconstruct source, commands, or an in-memory task inventory.
func (c *directCodingTaskCognition) CompleteSealedAppliedRecovery(
	operationID string,
	receiptSHA256 string,
) error {
	if c == nil {
		return fmt.Errorf("sealed applied recovery requires persisted task cognition")
	}
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	verification, ok := ledger.Node(c.verificationTaskID)
	_, verificationSuperseded := ledger.NodeSupersession(c.verificationTaskID)
	if !ok || verificationSuperseded || !directCodingRecoveredCompletionTask(
		verification, c.objectiveID, c.authority.StepID,
	) ||
		verification.Status != taskstate.NodeDone ||
		!directCodingRecoveredNodeCompletedByStep(verification, c.authority.StepID) ||
		len(verification.VerificationRefs) != 1 {
		return fmt.Errorf("sealed applied recovery lacks exact completed workspace verification cognition")
	}
	proof := verification.VerificationRefs[0]
	if proof.URI != fmt.Sprintf("verification://job/%d/workspace", c.authority.JobID) ||
		proof.Version != "v1" || proof.Relation != taskstate.RefVerifies ||
		!directCodingRollbackDockerIDPattern.MatchString(proof.Hash) {
		return fmt.Errorf("sealed applied recovery workspace verification proof is invalid")
	}
	children := make(map[taskstate.NodeID]taskstate.Node)
	for _, node := range ledger.Nodes() {
		if node.ParentID == c.objectiveID || node.ObjectiveID == c.objectiveID {
			children[node.ID] = node
		}
	}
	for _, edge := range ledger.Edges() {
		if edge.Kind != taskstate.EdgeDecomposes || edge.From != c.objectiveID {
			continue
		}
		node, exists := ledger.Node(edge.To)
		if !exists {
			return fmt.Errorf("sealed applied recovery decomposition references missing task %q", edge.To)
		}
		children[node.ID] = node
	}
	if len(children) == 0 {
		return fmt.Errorf("sealed applied recovery objective has no persisted task children")
	}
	deployment, hasDeployment := children[c.deploymentTaskID]
	_, deploymentSuperseded := ledger.NodeSupersession(c.deploymentTaskID)
	if !hasDeployment || deploymentSuperseded {
		return fmt.Errorf("sealed applied recovery lacks persisted deployment cognition")
	}
	deploymentProof := taskstate.Ref{
		URI: "deployment://" + operationID, Version: "v1", Hash: receiptSHA256,
		Relation: taskstate.RefVerifies,
	}
	if deployment.Status == taskstate.NodeDone &&
		!directCodingCompletionProofEqual(deployment.VerificationRefs, deploymentProof) {
		return fmt.Errorf("sealed applied recovery deployment proof conflicts with receipt")
	}
	if deployment.Status == taskstate.NodeActive && len(deployment.VerificationRefs) != 0 {
		return fmt.Errorf("sealed applied recovery active deployment carries terminal proof")
	}
	for _, node := range children {
		if _, superseded := ledger.NodeSupersession(node.ID); superseded {
			continue
		}
		if !directCodingRecoveredCompletionTask(node, c.objectiveID, c.authority.StepID) {
			return fmt.Errorf("sealed applied recovery task %q has invalid objective authority", node.ID)
		}
		if node.ID == c.deploymentTaskID {
			if node.Status != taskstate.NodeActive && node.Status != taskstate.NodeDone {
				return fmt.Errorf("sealed applied recovery deployment cognition is not active or complete")
			}
			if node.Status == taskstate.NodeDone &&
				!directCodingRecoveredNodeCompletedByStep(node, c.authority.StepID) {
				return fmt.Errorf("sealed applied recovery deployment completion step differs")
			}
			continue
		}
		if node.Status != taskstate.NodeDone ||
			!directCodingRecoveredNodeCompletedByStep(node, c.authority.StepID) {
			return fmt.Errorf("sealed applied recovery task %q is not exactly complete", node.ID)
		}
	}
	objective, ok := ledger.Node(c.objectiveID)
	_, objectiveSuperseded := ledger.NodeSupersession(c.objectiveID)
	if !ok || objectiveSuperseded || objective.Kind != taskstate.NodeObjective ||
		objective.ParentID != "goal:root" || objective.ObjectiveID != "" ||
		objective.CreatedBy != taskstate.AuthorityCode || objective.CreatedStepID == nil ||
		*objective.CreatedStepID != c.authority.StepID || objective.AssignedStepID != nil ||
		objective.Status != taskstate.NodeActive &&
			objective.Status != taskstate.NodeDone {
		return fmt.Errorf("sealed applied recovery objective is not active or complete")
	}
	if objective.Status == taskstate.NodeDone {
		if deployment.Status != taskstate.NodeDone {
			return fmt.Errorf("sealed applied recovery objective completed before deployment cognition")
		}
		if !directCodingRecoveredNodeCompletedByStep(objective, c.authority.StepID) {
			return fmt.Errorf("sealed applied recovery objective completion step differs")
		}
		if !directCodingCompletionProofEqual(objective.VerificationRefs, proof) {
			return fmt.Errorf("sealed applied recovery objective proof conflicts with workspace verification")
		}
	}
	if err := c.validateSealedAppliedWorkingSet(
		ledger, objective, verification, deployment, children,
	); err != nil {
		return err
	}
	if _, err := c.CompleteDeployment(operationID, receiptSHA256); err != nil {
		return fmt.Errorf("complete sealed deployment cognition: %w", err)
	}
	if objective.Status != taskstate.NodeDone {
		stepID := c.authority.StepID
		if err := c.transition(c.objectiveID, taskstate.NodeDone, &stepID, []taskstate.Ref{proof}); err != nil {
			return err
		}
	}
	return c.closeScope(
		workingset.Scope{Kind: workingset.ScopeObjective, ID: workingset.ScopeID(c.objectiveID)},
		"sealed workspace verification and persistent deployment receipts recovered",
	)
}

func directCodingRecoveredNodeCompletedByStep(node taskstate.Node, stepID int64) bool {
	return node.CompletedStepID != nil && *node.CompletedStepID == stepID
}

func directCodingRecoveredCompletionTask(
	node taskstate.Node,
	objectiveID taskstate.NodeID,
	stepID int64,
) bool {
	return node.ParentID == objectiveID && node.ObjectiveID == objectiveID &&
		node.Kind == taskstate.NodeTask && node.InlineExecution &&
		node.CreatedBy == taskstate.AuthorityCode && node.CreatedStepID != nil &&
		*node.CreatedStepID == stepID && node.AssignedStepID == nil
}
