package assemblyline

import (
	"fmt"
	"strings"
)

const DatabaseSchemaRelationInventorySchemaV1 = "omnidex.database-schema-relation-inventory.v1"

type DatabaseSchemaRelationInventory struct {
	Schema          string   `json:"schema"`
	AuthoritySHA256 string   `json:"authority_sha256"`
	RawSHA256       string   `json:"raw_sha256"`
	Candidates      []string `json:"candidates"`
}

func NewDatabaseSchemaRelationInventoryJob(
	input DatabaseSchemaRelationInventoryInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkDatabaseSchemaRelationInventory,
		input,
		input.validate,
	)
}

func BuildDatabaseSchemaRelationInventoryPrompt(
	input DatabaseSchemaRelationInventoryInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	objective, err := renderDatabaseSchemaObjective(input.ExactNeed, input.Context)
	if err != nil {
		return "", err
	}
	options, err := renderDatabaseSchemaRelationOptions(input.Candidates, false)
	if err != nil {
		return "", err
	}
	prompt := strings.Join([]string{
		"Return one bounded minimal inventory of candidate semantic relation responsibilities from the available relation descriptors that might be necessary to answer the exact database objective.",
		"Each line describes only what information one relation would need to provide for the exact objective. Include no customary, optional, speculative, or merely useful responsibility.",
		fmt.Sprintf("Return between 1 and %d concise raw candidate lines, with no blank lines, JSON, labels, Markdown, explanation, or surrounding envelope.", MaxDatabaseSchemaRelationInventoryCandidates),
		"EXACT DATABASE OBJECTIVE:\n" + objective,
		"AVAILABLE REGISTERED RELATION DESCRIPTORS (IDENTIFIERS WITHHELD):\n" + options,
		"FINAL QUESTION:\nWhat bounded candidate relation-responsibility inventory might be necessary for this exact database objective? Return only one candidate purpose per non-empty line.",
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("database schema relation inventory prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func DecodeDatabaseSchemaRelationInventory(
	input DatabaseSchemaRelationInventoryInput,
	raw string,
) (DatabaseSchemaRelationInventory, error) {
	var zero DatabaseSchemaRelationInventory
	if err := input.validate(); err != nil {
		return zero, err
	}
	inventoryText, err := decodeRawSemanticLeaf(
		"database schema relation inventory",
		raw,
		maxDatabaseSchemaRelationInventoryBytes,
		true,
	)
	if err != nil {
		return zero, err
	}
	if strings.ContainsRune(inventoryText, '\r') {
		return zero, fmt.Errorf("database schema relation inventory must use LF line boundaries")
	}
	candidates := strings.Split(inventoryText, "\n")
	maximum := MaxDatabaseSchemaRelationInventoryCandidates
	if len(candidates) < 1 || len(candidates) > maximum {
		return zero, fmt.Errorf(
			"database schema relation inventory must contain between 1 and %d candidate lines",
			maximum,
		)
	}
	for index, candidate := range candidates {
		leaf, err := decodeRawSemanticLeaf(
			fmt.Sprintf("database schema relation inventory candidate %d", index+1),
			candidate,
			maxDatabaseSchemaRelationCandidateBytes,
			false,
		)
		if err != nil {
			return zero, err
		}
		if err := validateDatabaseSchemaRelationCandidatePurpose(leaf); err != nil {
			return zero, fmt.Errorf("database schema relation inventory candidate %d: %w", index+1, err)
		}
		candidates[index] = leaf
	}
	authoritySHA256, err := databaseSchemaSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return zero, err
	}
	result := DatabaseSchemaRelationInventory{
		Schema:          DatabaseSchemaRelationInventorySchemaV1,
		AuthoritySHA256: authoritySHA256,
		RawSHA256:       ExactObjectiveContextSHA(strings.Join(candidates, "\n")),
		Candidates:      append([]string(nil), candidates...),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func (inventory DatabaseSchemaRelationInventory) ValidateFor(
	input DatabaseSchemaRelationInventoryInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if inventory.Schema != DatabaseSchemaRelationInventorySchemaV1 {
		return fmt.Errorf("database schema relation inventory schema must be %q", DatabaseSchemaRelationInventorySchemaV1)
	}
	authoritySHA256, err := databaseSchemaSemanticAuthoritySHA256(input, input.validate)
	if err != nil {
		return err
	}
	if inventory.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("database schema relation inventory authority hash does not match")
	}
	maximum := MaxDatabaseSchemaRelationInventoryCandidates
	if inventory.Candidates == nil || len(inventory.Candidates) < 1 || len(inventory.Candidates) > maximum {
		return fmt.Errorf("database schema relation inventory must contain between 1 and %d candidates", maximum)
	}
	for index, candidate := range inventory.Candidates {
		if strings.ContainsAny(candidate, "\r\n") {
			return fmt.Errorf("database schema relation inventory candidate %d must be one line", index+1)
		}
		if err := validateDatabaseSchemaRelationCandidatePurpose(candidate); err != nil {
			return fmt.Errorf("database schema relation inventory candidate %d: %w", index+1, err)
		}
	}
	if inventory.RawSHA256 != ExactObjectiveContextSHA(strings.Join(inventory.Candidates, "\n")) {
		return fmt.Errorf("database schema relation inventory raw hash does not match")
	}
	return nil
}
