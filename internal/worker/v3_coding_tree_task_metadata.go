package worker

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/taskstate"
)

const directCodingTreeTaskMetadataSchema = "omnidex.direct-coding-tree-task.v1"

type directCodingTreeTaskMetadata struct {
	Schema string                                `json:"schema"`
	Kind   assemblyline.TargetTreeTransitionKind `json:"kind"`
	Path   string                                `json:"path"`
}

func newDirectCodingTreeTaskMetadata(
	transition assemblyline.TargetTreeTransition,
) (taskstate.JSONObject, error) {
	key, err := directCodingTreeTaskKey(transition)
	if err != nil {
		return taskstate.JSONObject{}, err
	}
	normalized := assemblyline.TargetTreeTransition{
		Kind: transition.Kind,
		Path: key[len(string(transition.Kind))+1:],
	}
	raw, err := json.Marshal(directCodingTreeTaskMetadata{
		Schema: directCodingTreeTaskMetadataSchema,
		Kind:   normalized.Kind,
		Path:   normalized.Path,
	})
	if err != nil {
		return taskstate.JSONObject{}, fmt.Errorf("encode direct-coding tree task metadata: %w", err)
	}
	return taskstate.NewJSONObject(raw)
}

func directCodingTreeTransitionFromNode(
	node taskstate.Node,
	objectiveID taskstate.NodeID,
	stepID int64,
) (assemblyline.TargetTreeTransition, error) {
	if objectiveID == "" || stepID <= 0 ||
		node.Kind != taskstate.NodeTask || !node.InlineExecution ||
		node.ParentID != objectiveID || node.ObjectiveID != objectiveID ||
		node.CreatedBy != taskstate.AuthorityCode || node.CreatedStepID == nil ||
		*node.CreatedStepID != stepID || node.AssignedStepID != nil {
		return assemblyline.TargetTreeTransition{}, fmt.Errorf(
			"persisted direct-coding tree task %q has invalid code authority", node.ID,
		)
	}
	decoder := json.NewDecoder(bytes.NewReader(node.Metadata.Bytes()))
	decoder.DisallowUnknownFields()
	var metadata directCodingTreeTaskMetadata
	if err := decoder.Decode(&metadata); err != nil {
		return assemblyline.TargetTreeTransition{}, fmt.Errorf(
			"decode persisted direct-coding tree task %q: %w", node.ID, err,
		)
	}
	if err := requireDirectCodingTreeMetadataEOF(decoder); err != nil {
		return assemblyline.TargetTreeTransition{}, fmt.Errorf(
			"decode persisted direct-coding tree task %q: %w", node.ID, err,
		)
	}
	if metadata.Schema != directCodingTreeTaskMetadataSchema {
		return assemblyline.TargetTreeTransition{}, fmt.Errorf(
			"persisted direct-coding tree task %q has unsupported metadata schema", node.ID,
		)
	}
	transition := assemblyline.TargetTreeTransition{Kind: metadata.Kind, Path: metadata.Path}
	key, err := directCodingTreeTaskKey(transition)
	if err != nil {
		return assemblyline.TargetTreeTransition{}, fmt.Errorf(
			"persisted direct-coding tree task %q metadata: %w", node.ID, err,
		)
	}
	if transition.Path != metadata.Path || node.ID != taskstate.NodeID(
		"direct-coding-tree-"+directCodingDigest(key),
	) {
		return assemblyline.TargetTreeTransition{}, fmt.Errorf(
			"persisted direct-coding tree task %q identity differs from metadata", node.ID,
		)
	}
	title, criterion, err := directCodingTreeTaskDescription(transition)
	if err != nil {
		return assemblyline.TargetTreeTransition{}, err
	}
	if node.Title != title || node.Priority != 40 ||
		len(node.AcceptanceCriteria) != 1 || node.AcceptanceCriteria[0] != criterion {
		return assemblyline.TargetTreeTransition{}, fmt.Errorf(
			"persisted direct-coding tree task %q presentation differs from metadata", node.ID,
		)
	}
	return transition, nil
}

func requireDirectCodingTreeMetadataEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("metadata contains trailing JSON")
		}
		return fmt.Errorf("metadata trailing JSON: %w", err)
	}
	return nil
}
