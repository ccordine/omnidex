package worker

import (
	"fmt"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

// PlanTreeTransitions compiles the already accepted path-only tree diff into
// inline leaf tasks. It never asks a model what to create, order, or run.
func (c *directCodingTaskCognition) PlanTreeTransitions(transitions []assemblyline.TargetTreeTransition) error {
	if c == nil || len(transitions) == 0 {
		return fmt.Errorf("direct coding task cognition requires one non-empty target-tree diff")
	}
	for _, transition := range transitions {
		key, err := directCodingTreeTaskKey(transition)
		if err != nil {
			return err
		}
		id := c.treeTaskNodeID(key)
		c.treeTaskIDs[key] = id
		switch transition.Kind {
		case assemblyline.TargetTreeEnsureDirectory:
			c.treeDirs[transition.Path] = transition
		case assemblyline.TargetTreeCreate, assemblyline.TargetTreeReconcile:
			c.treeFiles[transition.Path] = transition
		}
		ledger, err := c.ledger()
		if err != nil {
			return err
		}
		if existing, exists := ledger.Node(id); exists {
			if existing.Kind != taskstate.NodeTask || !existing.InlineExecution || existing.ParentID != c.objectiveID || existing.ObjectiveID != c.objectiveID {
				return fmt.Errorf("direct coding task cognition found incompatible persisted tree leaf %q", id)
			}
			continue
		}
		if err := c.addTreeTask(transition, key, id); err != nil {
			return err
		}
	}
	for _, transition := range transitions {
		if err := c.addTreeDependencies(transition); err != nil {
			return err
		}
	}
	return c.promoteReady()
}

func (c *directCodingTaskCognition) BeginTreeDirectory(path string) error {
	transition, ok := c.treeDirs[path]
	if !ok {
		return fmt.Errorf("direct coding task cognition directory %q is not planned", path)
	}
	return c.BeginTreeTransition(transition)
}

func (c *directCodingTaskCognition) CompleteTreeDirectory(path, evidence string) error {
	transition, ok := c.treeDirs[path]
	if !ok {
		return fmt.Errorf("direct coding task cognition directory %q is not planned", path)
	}
	return c.CompleteTreeTransition(transition, evidence)
}

func (c *directCodingTaskCognition) BeginTreeFile(path string) error {
	transition, ok := c.treeFiles[path]
	if !ok {
		return fmt.Errorf("direct coding task cognition file %q is not planned", path)
	}
	return c.BeginTreeTransition(transition)
}

func (c *directCodingTaskCognition) CompleteTreeFile(path, evidence string) error {
	transition, ok := c.treeFiles[path]
	if !ok {
		return fmt.Errorf("direct coding task cognition file %q is not planned", path)
	}
	return c.CompleteTreeTransition(transition, evidence)
}

func (c *directCodingTaskCognition) BeginTreeTransition(transition assemblyline.TargetTreeTransition) error {
	key, err := directCodingTreeTaskKey(transition)
	if err != nil {
		return err
	}
	id, ok := c.treeTaskIDs[key]
	if !ok {
		return fmt.Errorf("direct coding task cognition tree leaf %q is not planned", transition.Path)
	}
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	next, runnable := ledger.NextRunnableNode()
	if !runnable || next.ID != id {
		return fmt.Errorf("direct coding task cognition cannot start tree leaf %q before its persisted prerequisites", transition.Path)
	}
	if err := c.transition(id, taskstate.NodeActive, nil, nil); err != nil {
		return err
	}
	return c.acquire("tree-"+directCodingDigest(key), workingset.RoleTask, workingset.RetentionTask,
		workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(id)},
		taskstate.Ref{URI: fmt.Sprintf("tree://job/%d/%s", c.authority.JobID, directCodingDigest(key)), Version: "v1", Hash: directCodingDigest(key), Relation: taskstate.RefSource},
		len(transition.Path), "retain active code-owned tree leaf")
}

func (c *directCodingTaskCognition) CompleteTreeTransition(transition assemblyline.TargetTreeTransition, evidence string) error {
	key, err := directCodingTreeTaskKey(transition)
	if err != nil {
		return err
	}
	id, ok := c.treeTaskIDs[key]
	if !ok {
		return fmt.Errorf("direct coding task cognition tree leaf %q is not planned", transition.Path)
	}
	if strings.TrimSpace(evidence) == "" {
		return fmt.Errorf("direct coding task cognition tree leaf %q has no code-owned evidence", transition.Path)
	}
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	node, ok := ledger.Node(id)
	if !ok || node.Status != taskstate.NodeActive {
		return fmt.Errorf("direct coding task cognition tree leaf %q is not active", transition.Path)
	}
	stepID := c.authority.StepID
	proof := taskstate.Ref{
		URI: fmt.Sprintf("filesystem://job/%d/%s", c.authority.JobID, directCodingDigest(key)), Version: "v1",
		Hash: directCodingDigest(evidence), Relation: taskstate.RefVerifies,
	}
	if err := c.transition(id, taskstate.NodeDone, &stepID, []taskstate.Ref{proof}); err != nil {
		return err
	}
	if err := c.closeScope(workingset.Scope{Kind: workingset.ScopeTask, ID: workingset.ScopeID(id)}, "code-owned filesystem transition verified"); err != nil {
		return err
	}
	return c.promoteReady()
}

func (c *directCodingTaskCognition) addTreeTask(transition assemblyline.TargetTreeTransition, key string, id taskstate.NodeID) error {
	title, criterion, err := directCodingTreeTaskDescription(transition)
	if err != nil {
		return err
	}
	stepID := c.authority.StepID
	if err := c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		commandID, err := c.commandID("add-tree-task", directCodingDigest(key))
		if err != nil {
			return nil, err
		}
		return taskstate.AddNodeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID: id, ParentID: c.objectiveID, ObjectiveID: c.objectiveID, Kind: taskstate.NodeTask,
			InlineExecution: true, Title: title, Priority: 40, CreatedStepID: &stepID,
			AcceptanceCriteria: []string{criterion}, Metadata: taskstate.EmptyJSONObject(),
		}, nil
	}); err != nil {
		return err
	}
	return c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		commandID, err := c.commandID("tree-decomposes", directCodingDigest(key))
		if err != nil {
			return nil, err
		}
		return taskstate.AddEdgeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID:   taskstate.EdgeID("direct-coding-tree-decomposes-" + directCodingDigest(key)),
			Kind: taskstate.EdgeDecomposes, From: c.objectiveID, To: id,
		}, nil
	})
}

func (c *directCodingTaskCognition) addTreeDependencies(transition assemblyline.TargetTreeTransition) error {
	key, err := directCodingTreeTaskKey(transition)
	if err != nil {
		return err
	}
	dependent := c.treeTaskIDs[key]
	if dependent == "" {
		return fmt.Errorf("direct coding task cognition tree leaf %q is not registered", transition.Path)
	}
	for taskID, prerequisite := range c.taskIDs {
		if err := c.addTreeDependency(dependent, prerequisite, "source-"+taskID+"-"+directCodingDigest(key)); err != nil {
			return err
		}
	}
	parent := directCodingTreeParentDirectory(transition)
	if parent == "" {
		return nil
	}
	parentKey, err := directCodingTreeTaskKey(assemblyline.TargetTreeTransition{Kind: assemblyline.TargetTreeEnsureDirectory, Path: parent})
	if err != nil {
		return err
	}
	if prerequisite := c.treeTaskIDs[parentKey]; prerequisite != "" {
		return c.addTreeDependency(dependent, prerequisite, "parent-"+directCodingDigest(key))
	}
	return nil
}

func (c *directCodingTaskCognition) addTreeDependency(dependent, prerequisite taskstate.NodeID, label string) error {
	return c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
		for _, edge := range ledger.Edges() {
			if edge.Kind == taskstate.EdgeDependsOn && edge.From == dependent && edge.To == prerequisite {
				return nil, nil
			}
		}
		commandID, err := c.commandID("tree-dependency", label)
		if err != nil {
			return nil, err
		}
		return taskstate.AddEdgeCommand{
			CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
			ID:   taskstate.EdgeID("direct-coding-tree-dependency-" + directCodingDigest(label)),
			Kind: taskstate.EdgeDependsOn, From: dependent, To: prerequisite,
		}, nil
	})
}

func directCodingTreeTaskKey(transition assemblyline.TargetTreeTransition) (string, error) {
	if transition.Kind != assemblyline.TargetTreeEnsureDirectory && transition.Kind != assemblyline.TargetTreeCreate && transition.Kind != assemblyline.TargetTreeReconcile {
		return "", fmt.Errorf("direct coding tree leaf has unsupported transition %q", transition.Kind)
	}
	pathValue, err := normalizeDirectCodingPath(transition.Path)
	if err != nil {
		return "", err
	}
	return string(transition.Kind) + "\x00" + pathValue, nil
}

func (c *directCodingTaskCognition) treeTaskNodeID(key string) taskstate.NodeID {
	return taskstate.NodeID("direct-coding-tree-" + directCodingDigest(key))
}

func directCodingTreeTaskDescription(transition assemblyline.TargetTreeTransition) (string, string, error) {
	pathValue, err := normalizeDirectCodingPath(transition.Path)
	if err != nil {
		return "", "", err
	}
	switch transition.Kind {
	case assemblyline.TargetTreeEnsureDirectory:
		return "Ensure directory " + pathValue, "The workspace contains directory " + pathValue + ".", nil
	case assemblyline.TargetTreeCreate:
		return "Create file " + pathValue, "The workspace contains file " + pathValue + ".", nil
	case assemblyline.TargetTreeReconcile:
		return "Reconcile file " + pathValue, "The workspace contains the verified file " + pathValue + ".", nil
	default:
		return "", "", fmt.Errorf("direct coding tree leaf has unsupported transition %q", transition.Kind)
	}
}

func directCodingTreeParentDirectory(transition assemblyline.TargetTreeTransition) string {
	if transition.Kind == assemblyline.TargetTreeEnsureDirectory {
		parent := path.Dir(transition.Path)
		if parent == "." {
			return ""
		}
		return parent
	}
	parent := path.Dir(transition.Path)
	if parent == "." {
		return ""
	}
	return parent
}
