package assemblyline

import "fmt"

const (
	DatabaseSchemaRelationChoiceSchemaV1 = "omnidex.database-schema-relation-choice.v1"

	databaseSchemaNoAdditionalRelationChoice = "code-owned:no-additional-relation"
	databaseSchemaRelationChoicePrefix       = "code-owned:remaining-relation:"
)

type DatabaseSchemaRelationChoiceResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	RelationID      string `json:"relation_id"`
	NoAdditional    bool   `json:"no_additional"`
}

func NewDatabaseSchemaRelationChoiceJob(
	input DatabaseSchemaRelationChoiceInput,
) (PortableJob, error) {
	return newPortableJob(WorkDatabaseSchemaRelationChoice, input)
}

func BuildDatabaseSchemaRelationChoicePrompt(
	input DatabaseSchemaRelationChoiceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	objective, err := renderDatabaseSchemaObjective(input.ExactNeed, input.Context)
	if err != nil {
		return "", err
	}
	choices, err := databaseSchemaRelationChoices(input)
	if err != nil {
		return "", err
	}
	prompt, err := RenderOpaqueModelChoiceQuestion(
		"Which one of these remaining relations, if any, is necessary to answer the database objective?",
		[]string{"Database objective:\n" + objective},
		choices,
	)
	if err != nil {
		return "", err
	}
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("database schema relation choice prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func DecodeDatabaseSchemaRelationChoiceResult(
	input DatabaseSchemaRelationChoiceInput,
	raw string,
) (DatabaseSchemaRelationChoiceResult, error) {
	var zero DatabaseSchemaRelationChoiceResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	choices, err := databaseSchemaRelationChoices(input)
	if err != nil {
		return zero, err
	}
	selected, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := databaseSchemaSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return zero, err
	}
	result := DatabaseSchemaRelationChoiceResult{
		Schema: DatabaseSchemaRelationChoiceSchemaV1, AuthoritySHA256: authoritySHA256,
	}
	if selected == databaseSchemaNoAdditionalRelationChoice {
		result.NoAdditional = true
	} else {
		for index, candidate := range input.Candidates {
			if selected == databaseSchemaRelationChoiceValue(index) {
				result.RelationID = candidate.RelationID
				break
			}
		}
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func databaseSchemaRelationChoices(
	input DatabaseSchemaRelationChoiceInput,
) ([]OpaqueModelChoice, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	specs := make([]databaseOpaqueChoiceSpec, 0, len(input.Candidates)+1)
	for index, candidate := range input.Candidates {
		specs = append(specs, databaseOpaqueChoiceSpec{
			description: candidate.Descriptor,
			value:       databaseSchemaRelationChoiceValue(index),
		})
	}
	specs = append(specs, databaseOpaqueChoiceSpec{
		description: "No additional remaining relation is necessary to answer the database objective",
		value:       databaseSchemaNoAdditionalRelationChoice,
	})
	return databaseOpaqueChoices(specs)
}

func databaseSchemaRelationChoiceValue(index int) string {
	return fmt.Sprintf("%s%d", databaseSchemaRelationChoicePrefix, index)
}

func (result DatabaseSchemaRelationChoiceResult) ValidateFor(
	input DatabaseSchemaRelationChoiceInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != DatabaseSchemaRelationChoiceSchemaV1 {
		return fmt.Errorf("database schema relation choice schema must be %q", DatabaseSchemaRelationChoiceSchemaV1)
	}
	authoritySHA256, err := databaseSchemaSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("database schema relation choice authority hash does not match")
	}
	if result.NoAdditional {
		if result.RelationID != "" {
			return fmt.Errorf("database schema no-additional choice cannot bind a relation ID")
		}
		return nil
	}
	if result.RelationID == "" {
		return fmt.Errorf("database schema relation choice did not bind a relation")
	}
	if !databaseSchemaRelationCandidateExists(input.Candidates, result.RelationID) {
		return fmt.Errorf("database schema relation ID %q was not projected", result.RelationID)
	}
	return nil
}

func databaseSchemaRelationCandidateExists(
	candidates []DatabaseSchemaCandidate,
	relationID string,
) bool {
	for _, candidate := range candidates {
		if candidate.RelationID == relationID {
			return true
		}
	}
	return false
}
