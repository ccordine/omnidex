package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	WorkApplicationStateFieldPurposeInventory   WorkKind = "application_state_field_purpose_inventory"
	WorkApplicationStateFieldKind               WorkKind = "application_state_field_kind"
	WorkApplicationRecordFieldPurposeInventory  WorkKind = "application_record_field_purpose_inventory"
	WorkApplicationRecordFieldKind              WorkKind = "application_record_field_kind"
	WorkApplicationServiceStatePurposeNecessity WorkKind = "application_service_state_purpose_necessity"
	WorkApplicationServiceStatePurposeRelation  WorkKind = "application_service_state_purpose_relation"

	MaxApplicationServiceStateInterfaceFields = maxApplicationServiceStateInterfaceFields
)

type ApplicationStateFieldKindInput struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	FocusedPurpose string                                `json:"focused_purpose"`
}

type ApplicationRecordFieldKindInput struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentPurpose  string                                `json:"parent_purpose"`
	FocusedPurpose string                                `json:"focused_purpose"`
}

func NewApplicationStateFieldKindJob(input ApplicationStateFieldKindInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationStateFieldKind, input, input.validate)
}

func NewApplicationRecordFieldKindJob(input ApplicationRecordFieldKindInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationRecordFieldKind, input, input.validate)
}

func (input ApplicationStateFieldKindInput) validate() error {
	if err := input.Authority.Validate(); err != nil {
		return err
	}
	return validateApplicationServiceStatePurpose("focused root field", input.FocusedPurpose)
}

func (input ApplicationRecordFieldKindInput) validate() error {
	if err := input.Authority.Validate(); err != nil {
		return err
	}
	if err := validateApplicationServiceStatePurpose("record parent", input.ParentPurpose); err != nil {
		return err
	}
	return validateApplicationServiceStatePurpose("focused record field", input.FocusedPurpose)
}

func validateApplicationServiceStateRecordField(field ApplicationServiceStateRecordField) error {
	if err := validateApplicationServiceStateFieldName(field.Name); err != nil {
		return err
	}
	if err := validateApplicationServiceStatePurpose("record field", field.Purpose); err != nil {
		return err
	}
	switch field.Kind {
	case ApplicationServiceStateString, ApplicationServiceStateInteger,
		ApplicationServiceStateNumber, ApplicationServiceStateBoolean:
		return nil
	default:
		return fmt.Errorf("record field %q has unsupported scalar kind %q", field.Name, field.Kind)
	}
}

func buildApplicationServiceStateLeafPrompt(
	validate func() error,
	question string,
	response string,
	label string,
	projection any,
) (string, error) {
	if err := validate(); err != nil {
		return "", err
	}
	authority, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application service state leaf authority: %w", err)
	}
	prompt := strings.Join([]string{
		question,
		response,
		"Return exactly the requested raw semantic result and nothing else.",
		label + ":\n" + string(authority),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application service state leaf prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}
