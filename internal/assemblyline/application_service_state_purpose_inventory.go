package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	ApplicationServiceStatePurposeInventorySchemaV1 = "omnidex.application-service-state-purpose-inventory.v1"
	maxApplicationServiceStatePurposeInventoryBytes = maxApplicationServiceStateInterfaceFields*MaxApplicationServiceStatePurposeBytes +
		maxApplicationServiceStateInterfaceFields - 1
)

type ApplicationStateFieldPurposeInventoryInput struct {
	Authority ApplicationServiceStateInterfaceInput `json:"authority"`
}

type ApplicationRecordFieldPurposeInventoryInput struct {
	Authority     ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentPurpose string                                `json:"parent_purpose"`
}

type ApplicationServiceStatePurposeInventory struct {
	Schema          string   `json:"schema"`
	AuthoritySHA256 string   `json:"authority_sha256"`
	RawSHA256       string   `json:"raw_sha256"`
	Purposes        []string `json:"purposes"`
}

func NewApplicationStateFieldPurposeInventoryJob(
	input ApplicationStateFieldPurposeInventoryInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationStateFieldPurposeInventory,
		input,
		input.validate,
	)
}

func NewApplicationRecordFieldPurposeInventoryJob(
	input ApplicationRecordFieldPurposeInventoryInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRecordFieldPurposeInventory,
		input,
		input.validate,
	)
}

func (input ApplicationStateFieldPurposeInventoryInput) validate() error {
	return input.Authority.Validate()
}

func (input ApplicationRecordFieldPurposeInventoryInput) validate() error {
	if err := input.Authority.Validate(); err != nil {
		return err
	}
	return validateApplicationServiceStatePurpose("record parent", input.ParentPurpose)
}

func BuildApplicationStateFieldPurposeInventoryPrompt(
	input ApplicationStateFieldPurposeInventoryInput,
) (string, error) {
	return buildApplicationServiceStatePurposeInventoryPrompt(
		input.validate,
		"Enumerate the minimal durable root-value purposes that might be necessary to implement the directly related accepted behavior authority.",
		"Each line states only one root value's durable domain responsibility.",
		"APPLICATION_STATE_FIELD_PURPOSE_INVENTORY_AUTHORITY",
		applicationStateFieldPurposeInventoryProjection{Authority: input.Authority},
	)
}

func BuildApplicationRecordFieldPurposeInventoryPrompt(
	input ApplicationRecordFieldPurposeInventoryInput,
) (string, error) {
	return buildApplicationServiceStatePurposeInventoryPrompt(
		input.validate,
		"Enumerate the minimal scalar member purposes that might be necessary within the focused durable record-list value to implement the directly related accepted behavior authority.",
		"Each line states only one scalar member's durable domain responsibility.",
		"APPLICATION_RECORD_FIELD_PURPOSE_INVENTORY_AUTHORITY",
		applicationRecordFieldPurposeInventoryProjection{
			Authority: input.Authority, ParentPurpose: input.ParentPurpose,
		},
	)
}

func buildApplicationServiceStatePurposeInventoryPrompt(
	validate func() error,
	question string,
	restriction string,
	label string,
	projection any,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		validate,
		question,
		strings.Join([]string{
			"Return each concise candidate purpose on one non-empty line. Preserve source meaning and omit customary capabilities not required by the supplied authority.",
			restriction,
			fmt.Sprintf("Return between 1 and %d raw candidate lines with no blank lines, JSON, quotes, labels, Markdown, explanation, or surrounding envelope.", maxApplicationServiceStateInterfaceFields),
		}, " "),
		label,
		projection,
	)
}

func DecodeApplicationStateFieldPurposeInventory(
	input ApplicationStateFieldPurposeInventoryInput,
	raw string,
) (ApplicationServiceStatePurposeInventory, error) {
	return decodeApplicationServiceStatePurposeInventory(input, raw, input.validate)
}

func DecodeApplicationRecordFieldPurposeInventory(
	input ApplicationRecordFieldPurposeInventoryInput,
	raw string,
) (ApplicationServiceStatePurposeInventory, error) {
	return decodeApplicationServiceStatePurposeInventory(input, raw, input.validate)
}

func decodeApplicationServiceStatePurposeInventory(
	input any,
	raw string,
	validate func() error,
) (ApplicationServiceStatePurposeInventory, error) {
	var zero ApplicationServiceStatePurposeInventory
	if err := validate(); err != nil {
		return zero, err
	}
	inventory, err := decodeRawSemanticLeaf(
		"application service state purpose inventory",
		raw,
		maxApplicationServiceStatePurposeInventoryBytes,
		true,
	)
	if err != nil {
		return zero, err
	}
	purposes, normalized, err := decodeApplicationServiceStatePurposeLines(inventory)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationServiceStatePurposeInventoryAuthoritySHA256(input, validate)
	if err != nil {
		return zero, err
	}
	result := ApplicationServiceStatePurposeInventory{
		Schema:          ApplicationServiceStatePurposeInventorySchemaV1,
		AuthoritySHA256: authoritySHA256,
		RawSHA256:       ExactObjectiveContextSHA(normalized),
		Purposes:        append([]string(nil), purposes...),
	}
	if err := result.validateFor(input, validate); err != nil {
		return zero, err
	}
	return result, nil
}

func decodeApplicationServiceStatePurposeLines(raw string) ([]string, string, error) {
	if strings.ContainsRune(raw, '\r') {
		return nil, "", fmt.Errorf("application service state purpose inventory must use LF line boundaries")
	}
	purposes := strings.Split(raw, "\n")
	if len(purposes) < 1 || len(purposes) > maxApplicationServiceStateInterfaceFields {
		return nil, "", fmt.Errorf(
			"application service state purpose inventory must contain between 1 and %d candidate lines",
			maxApplicationServiceStateInterfaceFields,
		)
	}
	for index, purpose := range purposes {
		leaf, err := decodeRawSemanticLeaf(
			fmt.Sprintf("application service state purpose inventory candidate %d", index+1),
			purpose,
			MaxApplicationServiceStatePurposeBytes,
			false,
		)
		if err != nil {
			return nil, "", err
		}
		if err := validateApplicationServiceStatePurpose("inventory candidate", leaf); err != nil {
			return nil, "", fmt.Errorf("application service state purpose inventory candidate %d: %w", index+1, err)
		}
		purposes[index] = leaf
	}
	return append([]string(nil), purposes...), strings.Join(purposes, "\n"), nil
}

func (inventory ApplicationServiceStatePurposeInventory) ValidateForStateFields(
	input ApplicationStateFieldPurposeInventoryInput,
) error {
	return inventory.validateFor(input, input.validate)
}

func (inventory ApplicationServiceStatePurposeInventory) ValidateForRecordFields(
	input ApplicationRecordFieldPurposeInventoryInput,
) error {
	return inventory.validateFor(input, input.validate)
}

func (inventory ApplicationServiceStatePurposeInventory) validateFor(
	input any,
	validate func() error,
) error {
	if err := validate(); err != nil {
		return err
	}
	if inventory.Schema != ApplicationServiceStatePurposeInventorySchemaV1 {
		return fmt.Errorf("application service state purpose inventory schema must be %q", ApplicationServiceStatePurposeInventorySchemaV1)
	}
	authoritySHA256, err := applicationServiceStatePurposeInventoryAuthoritySHA256(input, validate)
	if err != nil {
		return err
	}
	if inventory.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application service state purpose inventory authority hash does not match")
	}
	if inventory.Purposes == nil || len(inventory.Purposes) < 1 || len(inventory.Purposes) > maxApplicationServiceStateInterfaceFields {
		return fmt.Errorf("application service state purpose inventory must contain between 1 and %d candidates", maxApplicationServiceStateInterfaceFields)
	}
	for index, purpose := range inventory.Purposes {
		if err := validateApplicationServiceStatePurpose("inventory candidate", purpose); err != nil {
			return fmt.Errorf("application service state purpose inventory candidate %d: %w", index+1, err)
		}
	}
	if inventory.RawSHA256 != ExactObjectiveContextSHA(strings.Join(inventory.Purposes, "\n")) {
		return fmt.Errorf("application service state purpose inventory raw hash does not match")
	}
	return nil
}

func applicationServiceStatePurposeInventoryAuthoritySHA256(
	input any,
	validate func() error,
) (string, error) {
	if err := validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application service state purpose inventory authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
