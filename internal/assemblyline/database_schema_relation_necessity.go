package assemblyline

import (
	"fmt"
	"strings"
)

const (
	DatabaseSchemaRelationNecessary    = "NECESSARY_FOR_DATABASE_OBJECTIVE"
	DatabaseSchemaRelationNotNecessary = "NOT_NECESSARY_FOR_DATABASE_OBJECTIVE"

	DatabaseSchemaRelationNecessitySchemaV1 = "omnidex.database-schema-relation-necessity.v1"
)

type DatabaseSchemaRelationNecessityResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

func NewDatabaseSchemaRelationNecessityJob(
	input DatabaseSchemaRelationNecessityInput,
) (PortableJob, error) {
	return newPortableJob(
		WorkDatabaseSchemaRelationNecessity,
		input,
	)
}

func BuildDatabaseSchemaRelationNecessityPrompt(
	input DatabaseSchemaRelationNecessityInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	objective, err := renderDatabaseSchemaObjective(input.ExactNeed, input.Context)
	if err != nil {
		return "", err
	}
	prompt := strings.Join([]string{
		"Answer one semantic relation question: is the exact candidate relation responsibility necessary to answer the exact database objective?",
		"Return NECESSARY_FOR_DATABASE_OBJECTIVE only when the objective requires information supplied by that exact relation responsibility. Return NOT_NECESSARY_FOR_DATABASE_OBJECTIVE for an optional, customary, speculative, presentation-only, implementation-only, or merely useful responsibility.",
		"Evaluate only this candidate's necessity. Return only NECESSARY_FOR_DATABASE_OBJECTIVE or NOT_NECESSARY_FOR_DATABASE_OBJECTIVE, with no JSON, label, Markdown, or explanation.",
		"EXACT DATABASE OBJECTIVE:\n" + objective,
		"EXACT RELATION-RESPONSIBILITY CANDIDATE:\n" + input.Candidate,
		"FINAL QUESTION:\nIs this exact relation responsibility necessary for the exact database objective? Return only the registered relation.",
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("database schema relation necessity prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func DecodeDatabaseSchemaRelationNecessityResult(
	input DatabaseSchemaRelationNecessityInput,
	raw string,
) (DatabaseSchemaRelationNecessityResult, error) {
	var zero DatabaseSchemaRelationNecessityResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"database schema relation necessity",
		raw,
		maximumStringBytes(
			DatabaseSchemaRelationNecessary,
			DatabaseSchemaRelationNotNecessary,
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := databaseSchemaSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return zero, err
	}
	result := DatabaseSchemaRelationNecessityResult{
		Schema:          DatabaseSchemaRelationNecessitySchemaV1,
		AuthoritySHA256: authoritySHA256,
		Relation:        leaf,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (result DatabaseSchemaRelationNecessityResult) ValidateFor(
	input DatabaseSchemaRelationNecessityInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != DatabaseSchemaRelationNecessitySchemaV1 {
		return fmt.Errorf("database schema relation necessity schema must be %q", DatabaseSchemaRelationNecessitySchemaV1)
	}
	authoritySHA256, err := databaseSchemaSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("database schema relation necessity authority hash does not match")
	}
	switch result.Relation {
	case DatabaseSchemaRelationNecessary, DatabaseSchemaRelationNotNecessary:
		return nil
	default:
		return fmt.Errorf("database schema relation necessity value %q is not registered", result.Relation)
	}
}
