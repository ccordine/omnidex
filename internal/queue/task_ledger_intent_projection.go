package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/taskstate"
)

func buildAcceptedIntentProjection(
	source acceptedIntentProjection,
	intent artifacts.IntentArtifact,
) (acceptedIntentProjection, []taskstate.Command, error) {
	if source.ArtifactID <= 0 || source.JobID <= 0 || source.StepID <= 0 ||
		source.JobGeneration <= 0 || source.LedgerID == "" || source.PayloadSHA256 == "" {
		return acceptedIntentProjection{}, nil, fmt.Errorf("accepted intent projection source is incomplete")
	}
	if len(intent.Objectives) != 1 {
		return acceptedIntentProjection{}, nil, fmt.Errorf("accepted intent projection requires exactly one objective")
	}
	objective := intent.Objectives[0]
	source.ObjectiveNodeID = taskstate.NodeID(acceptedIntentOpaqueID(
		acceptedIntentObjectivePrefix, source, "objective", 0,
	))
	metadata, err := acceptedIntentMetadata(source, "objective", 0)
	if err != nil {
		return acceptedIntentProjection{}, nil, err
	}
	commands := make([]taskstate.Command, 0, 4+len(intent.Constraints)+len(intent.Ambiguities))
	version := source.LedgerStart
	addNodeID, err := acceptedIntentCommandID(source, "add-objective", 0)
	if err != nil {
		return acceptedIntentProjection{}, nil, err
	}
	commands = append(commands, taskstate.AddNodeCommand{
		CommandID: addNodeID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		ID: source.ObjectiveNodeID, ParentID: initialTaskRootNodeID,
		Kind: taskstate.NodeObjective, Title: objective.Description,
		Priority: objective.Priority, CreatedStepID: &source.StepID,
		AcceptanceCriteria: append([]string(nil), objective.AcceptanceCriteria...),
		Metadata:           metadata,
	})
	version++
	edgeID := taskstate.EdgeID(acceptedIntentOpaqueID(acceptedIntentEdgePrefix, source, "objective", 0))
	addEdgeID, err := acceptedIntentCommandID(source, "decompose-objective", 0)
	if err != nil {
		return acceptedIntentProjection{}, nil, err
	}
	commands = append(commands, taskstate.AddEdgeCommand{
		CommandID: addEdgeID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		ID: edgeID, Kind: taskstate.EdgeDecomposes,
		From: initialTaskRootNodeID, To: source.ObjectiveNodeID,
	})
	version++
	source.Items = append(source.Items, acceptedIntentItem(source, "objective", 0, source.ObjectiveNodeID, ""))
	for ordinal, constraint := range intent.Constraints {
		var command taskstate.Command
		source, command, err = appendAcceptedIntentEntry(
			source, version, "constraint", ordinal, constraint,
			taskstate.EntryConstraint, taskstate.AuthorityCode,
		)
		if err != nil {
			return acceptedIntentProjection{}, nil, err
		}
		commands = append(commands, command)
		version++
	}
	for ordinal, ambiguity := range intent.Ambiguities {
		var command taskstate.Command
		source, command, err = appendAcceptedIntentEntry(
			source, version, "ambiguity", ordinal, ambiguity,
			taskstate.EntryQuestion, taskstate.AuthorityModelProposal,
		)
		if err != nil {
			return acceptedIntentProjection{}, nil, err
		}
		commands = append(commands, command)
		version++
	}
	promoteID, err := acceptedIntentCommandID(source, "promote-objective", 0)
	if err != nil {
		return acceptedIntentProjection{}, nil, err
	}
	commands = append(commands, taskstate.PromoteReadyNodesCommand{
		CommandID: promoteID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
	})
	version++
	activateID, err := acceptedIntentCommandID(source, "activate-objective", 0)
	if err != nil {
		return acceptedIntentProjection{}, nil, err
	}
	commands = append(commands, taskstate.TransitionNodeCommand{
		CommandID: activateID, ExpectedVersion: version, Actor: taskstate.AuthorityCode,
		NodeID: source.ObjectiveNodeID, To: taskstate.NodeActive,
	})
	source.LedgerEnd = version + 1
	return source, commands, nil
}

func appendAcceptedIntentEntry(
	source acceptedIntentProjection,
	version uint64,
	kind string,
	ordinal int,
	content string,
	entryKind taskstate.EntryKind,
	authority taskstate.Authority,
) (acceptedIntentProjection, taskstate.Command, error) {
	entryID := taskstate.EntryID(acceptedIntentOpaqueID(
		acceptedIntentEntryPrefix+kind+":", source, kind, ordinal,
	))
	metadata, err := acceptedIntentMetadata(source, kind, ordinal)
	if err != nil {
		return acceptedIntentProjection{}, nil, err
	}
	commandID, err := acceptedIntentCommandID(source, "add-"+kind, ordinal)
	if err != nil {
		return acceptedIntentProjection{}, nil, err
	}
	item := acceptedIntentItem(source, kind, ordinal, "", entryID)
	source.Items = append(source.Items, item)
	return source, taskstate.AddEntryCommand{
		CommandID: commandID, ExpectedVersion: version, Actor: authority,
		ID: entryID, ScopeNodeID: source.ObjectiveNodeID,
		Kind: entryKind, Content: content, CreatedStepID: &source.StepID,
		Metadata: metadata, Refs: []taskstate.Ref{item.sourceRef()},
	}, nil
}

func acceptedIntentItem(
	source acceptedIntentProjection,
	kind string,
	ordinal int,
	nodeID taskstate.NodeID,
	entryID taskstate.EntryID,
) acceptedIntentProjectionItem {
	return acceptedIntentProjectionItem{
		Kind: kind, Ordinal: ordinal, NodeID: nodeID, EntryID: entryID,
		SourceURI:     acceptedIntentSourceURI(source.JobID, source.ArtifactID, kind, ordinal),
		SourceVersion: "1", SourceSHA256: source.PayloadSHA256,
	}
}

func acceptedIntentMetadata(
	source acceptedIntentProjection,
	kind string,
	ordinal int,
) (taskstate.JSONObject, error) {
	raw, err := json.Marshal(map[string]any{
		"artifact_id": source.ArtifactID, "artifact_kind": artifacts.KindIntent,
		"artifact_ordinal": ordinal, "artifact_payload_sha256": source.PayloadSHA256,
		"artifact_version": "1", "item_kind": kind,
		"projection_schema": acceptedIntentProjectionSchema,
	})
	if err != nil {
		return taskstate.JSONObject{}, fmt.Errorf("encode accepted intent metadata: %w", err)
	}
	metadata, err := taskstate.NewJSONObject(raw)
	if err != nil {
		return taskstate.JSONObject{}, fmt.Errorf("validate accepted intent metadata: %w", err)
	}
	return metadata, nil
}

func acceptedIntentCommandID(
	source acceptedIntentProjection,
	operation string,
	ordinal int,
) (taskstate.CommandID, error) {
	return taskstate.NewCommandID(
		acceptedIntentProjectionSchema, strconv.FormatInt(source.JobID, 10),
		strconv.FormatInt(source.ArtifactID, 10), operation, strconv.Itoa(ordinal),
	)
}

func acceptedIntentOpaqueID(
	prefix string,
	source acceptedIntentProjection,
	kind string,
	ordinal int,
) string {
	parts := []string{
		acceptedIntentProjectionSchema, strconv.FormatInt(source.JobID, 10),
		strconv.FormatInt(source.ArtifactID, 10), kind, strconv.Itoa(ordinal),
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + hex.EncodeToString(digest[:])
}
