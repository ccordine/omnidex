package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkDatabaseSchemaRelationInventory  WorkKind = "database_schema_relation_inventory"
	WorkDatabaseSchemaRelationNecessity  WorkKind = "database_schema_relation_necessity"
	WorkDatabaseSchemaRelationResolution WorkKind = "database_schema_relation_resolution"

	MaxDatabaseSchemaRelationInventoryCandidates = maxDatabaseSchemaCandidates
	maxDatabaseSchemaRelationCandidateBytes      = 1024
	maxDatabaseSchemaRelationInventoryBytes      = MaxDatabaseSchemaRelationInventoryCandidates*maxDatabaseSchemaRelationCandidateBytes +
		MaxDatabaseSchemaRelationInventoryCandidates - 1
)

type DatabaseSchemaRelationInventoryInput struct {
	ExactNeed  string                    `json:"exact_need"`
	Context    ObjectiveContext          `json:"objective_context"`
	Candidates []DatabaseSchemaCandidate `json:"candidates"`
}

type DatabaseSchemaRelationNecessityInput struct {
	ExactNeed string           `json:"exact_need"`
	Context   ObjectiveContext `json:"objective_context"`
	Candidate string           `json:"candidate"`
}

type DatabaseSchemaRelationResolutionInput struct {
	Candidate  string                    `json:"candidate"`
	Candidates []DatabaseSchemaCandidate `json:"candidates"`
}

func ProjectDatabaseSchemaRelationInventoryInput(
	input DatabaseSchemaSelectionInput,
) (DatabaseSchemaRelationInventoryInput, error) {
	if err := input.validate(); err != nil {
		return DatabaseSchemaRelationInventoryInput{}, err
	}
	projected := DatabaseSchemaRelationInventoryInput{
		ExactNeed: input.ExactNeed,
		Context:   CloneObjectiveContext(input.Context),
		Candidates: append(
			[]DatabaseSchemaCandidate(nil), input.Candidates...,
		),
	}
	if err := projected.validate(); err != nil {
		return DatabaseSchemaRelationInventoryInput{}, err
	}
	return projected, nil
}

func (input DatabaseSchemaRelationInventoryInput) validate() error {
	if err := validateDatabaseSchemaObjective(input.ExactNeed, input.Context); err != nil {
		return err
	}
	return validateDatabaseSchemaRelationCandidates(input.Candidates)
}

func (input DatabaseSchemaRelationNecessityInput) validate() error {
	if err := validateDatabaseSchemaObjective(input.ExactNeed, input.Context); err != nil {
		return err
	}
	return validateDatabaseSchemaRelationCandidatePurpose(input.Candidate)
}

func (input DatabaseSchemaRelationResolutionInput) validate() error {
	if err := validateDatabaseSchemaRelationCandidatePurpose(input.Candidate); err != nil {
		return err
	}
	return validateDatabaseSchemaRelationCandidates(input.Candidates)
}

func validateDatabaseSchemaObjective(exactNeed string, context ObjectiveContext) error {
	if err := validateGroundedText(
		"database exact evidence need", exactNeed, maxGroundedRequirementBytes, false,
	); err != nil {
		return err
	}
	return context.Validate()
}

func validateDatabaseSchemaRelationCandidates(candidates []DatabaseSchemaCandidate) error {
	return validateDatabaseSchemaCandidates(candidates)
}

func validateDatabaseSchemaRelationCandidatePurpose(candidate string) error {
	return validateGroundedText(
		"database schema relation candidate purpose",
		candidate,
		maxDatabaseSchemaRelationCandidateBytes,
		false,
	)
}

func renderDatabaseSchemaObjective(exactNeed string, context ObjectiveContext) (string, error) {
	if err := validateDatabaseSchemaObjective(exactNeed, context); err != nil {
		return "", err
	}
	projection, err := marshalObjectiveContextInputForModel(struct {
		ExactNeed string           `json:"exact_need"`
		Context   ObjectiveContext `json:"objective_context"`
	}{ExactNeed: exactNeed, Context: context}, context)
	if err != nil {
		return "", err
	}
	return string(projection), nil
}

func renderDatabaseSchemaRelationOptions(
	candidates []DatabaseSchemaCandidate,
	includeIDs bool,
) (string, error) {
	if err := validateDatabaseSchemaRelationCandidates(candidates); err != nil {
		return "", err
	}
	var rendered strings.Builder
	for index, candidate := range candidates {
		if includeIDs {
			fmt.Fprintf(&rendered, "RELATION ID %s:\n%s\n", candidate.RelationID, candidate.Descriptor)
			continue
		}
		fmt.Fprintf(&rendered, "RELATION OPTION %d:\n%s\n", index+1, candidate.Descriptor)
	}
	return strings.TrimSuffix(rendered.String(), "\n"), nil
}

func databaseSchemaSemanticAuthoritySHA256(
	input any,
	validate func() error,
) (string, error) {
	if err := validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode database schema semantic authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
