package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const WorkDatabaseSchemaRelationChoice WorkKind = "database_schema_relation_choice"

type DatabaseSchemaRelationChoiceInput struct {
	ExactNeed  string                    `json:"exact_need"`
	Context    ObjectiveContext          `json:"objective_context"`
	Candidates []DatabaseSchemaCandidate `json:"candidates"`
}

func ProjectDatabaseSchemaRelationChoiceInput(
	input DatabaseSchemaSelectionInput,
	remaining []DatabaseSchemaCandidate,
) (DatabaseSchemaRelationChoiceInput, error) {
	if err := input.validate(); err != nil {
		return DatabaseSchemaRelationChoiceInput{}, err
	}
	if err := validateDatabaseSchemaRelationCandidates(remaining); err != nil {
		return DatabaseSchemaRelationChoiceInput{}, err
	}
	authority := make(map[string]string, len(input.Candidates))
	for _, candidate := range input.Candidates {
		authority[candidate.RelationID] = candidate.Descriptor
	}
	for _, candidate := range remaining {
		descriptor, exists := authority[candidate.RelationID]
		if !exists || descriptor != candidate.Descriptor {
			return DatabaseSchemaRelationChoiceInput{}, fmt.Errorf(
				"database schema remaining relation %q differs from its selection authority",
				candidate.RelationID,
			)
		}
	}
	projected := DatabaseSchemaRelationChoiceInput{
		ExactNeed: input.ExactNeed,
		Context:   CloneObjectiveContext(input.Context),
		Candidates: append(
			[]DatabaseSchemaCandidate(nil), remaining...,
		),
	}
	if err := projected.validate(); err != nil {
		return DatabaseSchemaRelationChoiceInput{}, err
	}
	return projected, nil
}

func (input DatabaseSchemaRelationChoiceInput) validate() error {
	if err := validateDatabaseSchemaObjective(input.ExactNeed, input.Context); err != nil {
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

func renderDatabaseSchemaObjective(exactNeed string, context ObjectiveContext) (string, error) {
	if err := validateDatabaseSchemaObjective(exactNeed, context); err != nil {
		return "", err
	}
	var rendered strings.Builder
	rendered.WriteString(exactNeed)
	for _, capsule := range context.Capsules {
		rendered.WriteString("\n\n")
		rendered.WriteString(capsule.Content)
	}
	return rendered.String(), nil
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
