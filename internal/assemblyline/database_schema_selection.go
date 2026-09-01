package assemblyline

import (
	"fmt"
)

const (
	DatabaseSchemaSelectionV1           = "omnidex.database-schema-selection.v1"
	maxDatabaseSchemaCandidates         = 24
	maxDatabaseSchemaSelections         = 8
	maxDatabaseSchemaCandidateTextBytes = 4 * 1024
	maxDatabaseSchemaCandidateSetBytes  = 48 * 1024
)

const MaxDatabaseSchemaCandidates = maxDatabaseSchemaCandidates

type DatabaseSchemaCandidate struct {
	RelationID string `json:"relation_id"`
	Descriptor string `json:"descriptor"`
}

type DatabaseSchemaSelectionInput struct {
	EvidenceNeedID       string                    `json:"evidence_need_id"`
	ExactNeed            string                    `json:"exact_need"`
	Context              ObjectiveContext          `json:"objective_context"`
	Candidates           []DatabaseSchemaCandidate `json:"candidates"`
	MaxSelections        int                       `json:"max_selections"`
	HasAcceptedRelations bool                      `json:"has_accepted_relations"`
}

type DatabaseSchemaSelectionDecision struct {
	Schema         string   `json:"schema"`
	EvidenceNeedID string   `json:"evidence_need_id"`
	RelationIDs    []string `json:"relation_ids"`
}

func (input DatabaseSchemaSelectionInput) validate() error {
	if err := validateGroundedID("database evidence need ID", input.EvidenceNeedID, maxGroundedRequirementIDBytes); err != nil {
		return err
	}
	if err := validateGroundedText("database exact evidence need", input.ExactNeed, maxGroundedRequirementBytes, false); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if err := validateDatabaseSchemaCandidates(input.Candidates); err != nil {
		return err
	}
	if input.MaxSelections < 1 || input.MaxSelections > maxDatabaseSchemaSelections || input.MaxSelections > len(input.Candidates) {
		return fmt.Errorf("database schema selection bound must be 1..%d and fit its candidates", maxDatabaseSchemaSelections)
	}
	return nil
}

func validateDatabaseSchemaCandidates(candidates []DatabaseSchemaCandidate) error {
	if len(candidates) < 1 || len(candidates) > maxDatabaseSchemaCandidates {
		return fmt.Errorf("database schema selection requires 1..%d relation candidates", maxDatabaseSchemaCandidates)
	}
	seen := make(map[string]struct{}, len(candidates))
	total := 0
	for index, candidate := range candidates {
		if err := validateGroundedID("database relation ID", candidate.RelationID, maxGroundedEvidenceIDBytes); err != nil {
			return fmt.Errorf("database schema candidate %d: %w", index, err)
		}
		if _, duplicate := seen[candidate.RelationID]; duplicate {
			return fmt.Errorf("database schema relation ID %q is duplicated", candidate.RelationID)
		}
		seen[candidate.RelationID] = struct{}{}
		if err := validateGroundedText("database relation descriptor", candidate.Descriptor, maxDatabaseSchemaCandidateTextBytes, false); err != nil {
			return fmt.Errorf("database schema candidate %s: %w", candidate.RelationID, err)
		}
		total += len(candidate.RelationID) + len(candidate.Descriptor)
	}
	if total > maxDatabaseSchemaCandidateSetBytes {
		return fmt.Errorf("database schema candidate set exceeds %d bytes", maxDatabaseSchemaCandidateSetBytes)
	}
	return nil
}

func (decision DatabaseSchemaSelectionDecision) ValidateFor(input DatabaseSchemaSelectionInput) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != DatabaseSchemaSelectionV1 {
		return fmt.Errorf("database schema selection schema must be %q", DatabaseSchemaSelectionV1)
	}
	if decision.EvidenceNeedID != input.EvidenceNeedID {
		return fmt.Errorf("database schema selection evidence need ID does not match its authority")
	}
	if decision.RelationIDs == nil {
		return fmt.Errorf("database schema selection relation IDs must be an explicit array")
	}
	if len(decision.RelationIDs) > input.MaxSelections {
		return fmt.Errorf("database schema selection exceeds its %d relation bound", input.MaxSelections)
	}
	available := make(map[string]struct{}, len(input.Candidates))
	for _, candidate := range input.Candidates {
		available[candidate.RelationID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decision.RelationIDs))
	for _, id := range decision.RelationIDs {
		if _, exists := available[id]; !exists {
			return fmt.Errorf("database schema relation ID %q was not projected", id)
		}
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("database schema relation ID %q is duplicated", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}

func AssembleDatabaseSchemaSelectionDecision(
	input DatabaseSchemaSelectionInput,
	relationIDs []string,
) (DatabaseSchemaSelectionDecision, error) {
	decision := DatabaseSchemaSelectionDecision{
		Schema: DatabaseSchemaSelectionV1, EvidenceNeedID: input.EvidenceNeedID,
		RelationIDs: append([]string{}, relationIDs...),
	}
	if err := decision.ValidateFor(input); err != nil {
		return DatabaseSchemaSelectionDecision{}, err
	}
	return decision, nil
}
