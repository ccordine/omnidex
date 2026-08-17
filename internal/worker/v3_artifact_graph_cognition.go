package worker

import (
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/gryph/omnidex/internal/workingset"
)

const directCodingArtifactGraphEntryID = taskstate.EntryID("direct-coding-artifact-graph")

// RecordArtifactGraph makes the complete code-owned relationship projection a
// durable objective fact before filesystem work starts. The graph is not model
// context by default; individual adapters later project only the interface
// facts required by one dependent source block.
func (c *directCodingTaskCognition) RecordArtifactGraph(graph assemblyline.ArtifactGraph) error {
	if c == nil {
		return fmt.Errorf("direct coding task cognition is required")
	}
	ordered := graph.Sorted()
	if err := ordered.Validate(); err != nil {
		return fmt.Errorf("artifact graph: %w", err)
	}
	contentBytes, err := json.Marshal(ordered)
	if err != nil {
		return fmt.Errorf("encode artifact graph evidence: %w", err)
	}
	content := string(contentBytes)
	ledger, err := c.ledger()
	if err != nil {
		return err
	}
	if existing, exists := ledger.Entry(directCodingArtifactGraphEntryID); exists {
		if existing.Kind != taskstate.EntryFact || existing.ScopeNodeID != c.objectiveID || existing.Content != content {
			return fmt.Errorf("direct coding task cognition found incompatible persisted artifact graph")
		}
	} else {
		stepID := c.authority.StepID
		if err := c.apply(func(ledger *taskstate.Ledger) (taskstate.Command, error) {
			commandID, err := c.commandID("record-artifact-graph", directCodingDigest(content))
			if err != nil {
				return nil, err
			}
			return taskstate.AddEntryCommand{
				CommandID: commandID, ExpectedVersion: ledger.Version(), Actor: taskstate.AuthorityCode,
				ID: directCodingArtifactGraphEntryID, ScopeNodeID: c.objectiveID, Kind: taskstate.EntryFact,
				Content: content, CreatedStepID: &stepID, Metadata: taskstate.EmptyJSONObject(),
				Refs: []taskstate.Ref{{
					URI:     fmt.Sprintf("artifact-graph://job/%d/objective", c.authority.JobID),
					Version: assemblyline.ArtifactGraphSchemaV1, Hash: directCodingDigest(content), Relation: taskstate.RefEvidence,
				}},
			}, nil
		}); err != nil {
			return err
		}
	}
	return c.acquire(
		"artifact-graph", workingset.RoleFact, workingset.RetentionObjective,
		workingset.Scope{Kind: workingset.ScopeObjective, ID: workingset.ScopeID(c.objectiveID)},
		taskstate.Ref{
			URI:     fmt.Sprintf("artifact-graph://job/%d/objective", c.authority.JobID),
			Version: assemblyline.ArtifactGraphSchemaV1, Hash: directCodingDigest(content), Relation: taskstate.RefEvidence,
		},
		len(content), "retain code-derived artifact relationships and interfaces",
	)
}

func (c *directCodingTaskCognition) addArtifactGraphDependencies(graph assemblyline.ArtifactGraph) error {
	byID := make(map[string]assemblyline.ArtifactGraphArtifact, len(graph.Artifacts))
	for _, artifact := range graph.Artifacts {
		byID[artifact.ID] = artifact
	}
	for _, relation := range graph.Relations {
		if !artifactRelationCreatesPrerequisite(relation.Kind) {
			continue
		}
		dependentArtifact := byID[relation.From]
		prerequisiteArtifact := byID[relation.To]
		dependentTransition, dependentExists := c.treeFiles[dependentArtifact.Path]
		prerequisiteTransition, prerequisiteExists := c.treeFiles[prerequisiteArtifact.Path]
		if !dependentExists || !prerequisiteExists {
			return fmt.Errorf(
				"artifact graph relation %s from %q to %q has no planned filesystem leaves",
				relation.Kind, dependentArtifact.Path, prerequisiteArtifact.Path,
			)
		}
		dependentKey, err := directCodingTreeTaskKey(dependentTransition)
		if err != nil {
			return err
		}
		prerequisiteKey, err := directCodingTreeTaskKey(prerequisiteTransition)
		if err != nil {
			return err
		}
		if err := c.addTreeDependency(
			c.treeTaskIDs[dependentKey], c.treeTaskIDs[prerequisiteKey],
			"artifact-"+string(relation.Kind)+"-"+directCodingDigest(dependentArtifact.Path+"\x00"+prerequisiteArtifact.Path),
		); err != nil {
			return err
		}
	}
	return nil
}

func artifactRelationCreatesPrerequisite(kind assemblyline.ArtifactRelationKind) bool {
	switch kind {
	case assemblyline.ArtifactRelationDependsOn,
		assemblyline.ArtifactRelationConsumes,
		assemblyline.ArtifactRelationCalls,
		assemblyline.ArtifactRelationComposes,
		assemblyline.ArtifactRelationRoutesTo,
		assemblyline.ArtifactRelationPersistsTo,
		assemblyline.ArtifactRelationDataSource,
		assemblyline.ArtifactRelationVerifies:
		return true
	default:
		return false
	}
}
