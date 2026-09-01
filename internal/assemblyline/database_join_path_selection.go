package assemblyline

import (
	"fmt"
)

const (
	DatabaseJoinPathSelectionV1 = "omnidex.database-join-path-selection.v1"
	maxDatabaseJoinCandidates   = 8
)

type DatabaseJoinPathCandidate struct {
	PathID     string `json:"path_id"`
	Descriptor string `json:"descriptor"`
}

type DatabaseJoinPathSelectionInput struct {
	EvidenceNeedID string                      `json:"evidence_need_id"`
	ExactNeed      string                      `json:"exact_need"`
	Context        ObjectiveContext            `json:"objective_context"`
	FromRelationID string                      `json:"from_relation_id"`
	ToRelationID   string                      `json:"to_relation_id"`
	Candidates     []DatabaseJoinPathCandidate `json:"candidates"`
}

type DatabaseJoinPathSelectionDecision struct {
	Schema         string `json:"schema"`
	EvidenceNeedID string `json:"evidence_need_id"`
	PathID         string `json:"path_id"`
}

func NewDatabaseJoinPathSelectionJob(input DatabaseJoinPathSelectionInput) (PortableJob, error) {
	return newPortableJob(WorkDatabaseJoinPathSelection, input)
}

func (input DatabaseJoinPathSelectionInput) validate() error {
	if err := validateGroundedID("database evidence need ID", input.EvidenceNeedID, maxGroundedRequirementIDBytes); err != nil {
		return err
	}
	if err := validateGroundedText("database exact evidence need", input.ExactNeed, maxGroundedRequirementBytes, false); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if err := validateGroundedID("database from relation ID", input.FromRelationID, maxGroundedEvidenceIDBytes); err != nil {
		return err
	}
	if err := validateGroundedID("database to relation ID", input.ToRelationID, maxGroundedEvidenceIDBytes); err != nil {
		return err
	}
	if input.FromRelationID == input.ToRelationID {
		return fmt.Errorf("database join-path ambiguity requires two distinct relations")
	}
	if len(input.Candidates) < 1 || len(input.Candidates) > maxDatabaseJoinCandidates {
		return fmt.Errorf("database join-path selection requires 1..%d candidates", maxDatabaseJoinCandidates)
	}
	seen := make(map[string]struct{}, len(input.Candidates))
	for index, candidate := range input.Candidates {
		if err := validateGroundedID("database join path ID", candidate.PathID, maxGroundedEvidenceIDBytes); err != nil {
			return fmt.Errorf("database join-path candidate %d: %w", index, err)
		}
		if _, duplicate := seen[candidate.PathID]; duplicate {
			return fmt.Errorf("database join path ID %q is duplicated", candidate.PathID)
		}
		seen[candidate.PathID] = struct{}{}
		if err := validateGroundedText("database join-path descriptor", candidate.Descriptor, maxDatabaseSchemaCandidateTextBytes, false); err != nil {
			return fmt.Errorf("database join-path candidate %s: %w", candidate.PathID, err)
		}
	}
	return nil
}

func (decision DatabaseJoinPathSelectionDecision) ValidateFor(input DatabaseJoinPathSelectionInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != DatabaseJoinPathSelectionV1 || decision.EvidenceNeedID != input.EvidenceNeedID {
		return fmt.Errorf("database join-path selection does not match its exact authority")
	}
	for _, candidate := range input.Candidates {
		if decision.PathID == candidate.PathID {
			return nil
		}
	}
	return fmt.Errorf("database join path ID %q was not projected", decision.PathID)
}

func DecodeDatabaseJoinPathSelectionDecision(
	input DatabaseJoinPathSelectionInput,
	raw string,
) (DatabaseJoinPathSelectionDecision, error) {
	if err := input.validate(); err != nil {
		return DatabaseJoinPathSelectionDecision{}, err
	}
	choices, err := databaseJoinPathSelectionChoices(input)
	if err != nil {
		return DatabaseJoinPathSelectionDecision{}, err
	}
	leaf, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return DatabaseJoinPathSelectionDecision{}, err
	}
	decision := DatabaseJoinPathSelectionDecision{
		Schema: DatabaseJoinPathSelectionV1, EvidenceNeedID: input.EvidenceNeedID, PathID: leaf,
	}
	if err := decision.ValidateFor(input); err != nil {
		return decision, err
	}
	return decision, nil
}

func BuildDatabaseJoinPathSelectionPrompt(input DatabaseJoinPathSelectionInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	objective, err := renderDatabaseSchemaObjective(input.ExactNeed, input.Context)
	if err != nil {
		return "", err
	}
	choices, err := databaseJoinPathSelectionChoices(input)
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		"Which described foreign-key relationship matches the exact evidence need?",
		[]string{"Database objective:\n" + objective},
		choices,
	)
}

func databaseJoinPathSelectionChoices(
	input DatabaseJoinPathSelectionInput,
) ([]OpaqueModelChoice, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	specs := make([]databaseOpaqueChoiceSpec, 0, len(input.Candidates))
	for _, candidate := range input.Candidates {
		specs = append(specs, databaseOpaqueChoiceSpec{
			description: candidate.Descriptor,
			value:       candidate.PathID,
		})
	}
	return databaseOpaqueChoices(specs)
}

func ResolveSoleDatabaseJoinPathSelectionDecision(
	input DatabaseJoinPathSelectionInput,
) (DatabaseJoinPathSelectionDecision, bool, error) {
	choices, err := databaseJoinPathSelectionChoices(input)
	if err != nil {
		return DatabaseJoinPathSelectionDecision{}, false, err
	}
	pathID, resolved, err := resolveSoleDatabaseOpaqueChoice(choices)
	if err != nil || !resolved {
		return DatabaseJoinPathSelectionDecision{}, resolved, err
	}
	decision := DatabaseJoinPathSelectionDecision{
		Schema:         DatabaseJoinPathSelectionV1,
		EvidenceNeedID: input.EvidenceNeedID,
		PathID:         pathID,
	}
	if err := decision.ValidateFor(input); err != nil {
		return DatabaseJoinPathSelectionDecision{}, false, err
	}
	return decision, true, nil
}
