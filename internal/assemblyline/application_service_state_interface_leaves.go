package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	WorkApplicationStateFieldCoverage  WorkKind = "application_state_field_coverage"
	WorkApplicationStateFieldPurpose   WorkKind = "application_state_field_purpose"
	WorkApplicationStateFieldKind      WorkKind = "application_state_field_kind"
	WorkApplicationRecordFieldCoverage WorkKind = "application_record_field_coverage"
	WorkApplicationRecordFieldPurpose  WorkKind = "application_record_field_purpose"
	WorkApplicationRecordFieldKind     WorkKind = "application_record_field_kind"

	ApplicationStateFieldRemains      = "STATE_FIELD_REMAINS"
	ApplicationNoUncoveredStateField  = "NO_UNCOVERED_STATE_FIELD"
	ApplicationRecordFieldRemains     = "RECORD_FIELD_REMAINS"
	ApplicationNoUncoveredRecordField = "NO_UNCOVERED_RECORD_FIELD"

	MaxApplicationServiceStateInterfaceFields = maxApplicationServiceStateInterfaceFields
)

type ApplicationStateFieldLeafInput struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	AcceptedFields []ApplicationServiceStateField        `json:"accepted_fields"`
}

type ApplicationStateFieldKindInput struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	AcceptedFields []ApplicationServiceStateField        `json:"accepted_fields"`
	FocusedPurpose string                                `json:"focused_purpose"`
}

type ApplicationRecordFieldLeafInput struct {
	Authority            ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentPurpose        string                                `json:"parent_purpose"`
	AcceptedRecordFields []ApplicationServiceStateRecordField  `json:"accepted_record_fields"`
}

type ApplicationRecordFieldKindInput struct {
	Authority            ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentPurpose        string                                `json:"parent_purpose"`
	AcceptedRecordFields []ApplicationServiceStateRecordField  `json:"accepted_record_fields"`
	FocusedPurpose       string                                `json:"focused_purpose"`
}

func NewApplicationStateFieldCoverageJob(input ApplicationStateFieldLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationStateFieldCoverage, input, input.validate)
}

func NewApplicationStateFieldPurposeJob(input ApplicationStateFieldLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationStateFieldPurpose, input, input.validate)
}

func NewApplicationStateFieldKindJob(input ApplicationStateFieldKindInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationStateFieldKind, input, input.validate)
}

func NewApplicationRecordFieldCoverageJob(input ApplicationRecordFieldLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationRecordFieldCoverage, input, input.validate)
}

func NewApplicationRecordFieldPurposeJob(input ApplicationRecordFieldLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationRecordFieldPurpose, input, input.validate)
}

func NewApplicationRecordFieldKindJob(input ApplicationRecordFieldKindInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationRecordFieldKind, input, input.validate)
}

func (input ApplicationStateFieldLeafInput) validate() error {
	if err := input.Authority.Validate(); err != nil {
		return err
	}
	if input.AcceptedFields == nil {
		return fmt.Errorf("application state field accepted set must be non-nil")
	}
	if len(input.AcceptedFields) > maxApplicationServiceStateInterfaceFields {
		return fmt.Errorf(
			"application state field accepted set exceeds %d fields",
			maxApplicationServiceStateInterfaceFields,
		)
	}
	if len(input.AcceptedFields) == 0 {
		return nil
	}
	return (ApplicationServiceStateInterfaceResult{
		Schema: ApplicationServiceStateInterfaceSchemaV2,
		Fields: input.AcceptedFields,
	}).ValidateFor(input.Authority)
}

func (input ApplicationStateFieldKindInput) validate() error {
	if err := (ApplicationStateFieldLeafInput{
		Authority: input.Authority, AcceptedFields: input.AcceptedFields,
	}).validate(); err != nil {
		return err
	}
	if err := validateApplicationServiceStatePurpose("focused root field", input.FocusedPurpose); err != nil {
		return err
	}
	for _, field := range input.AcceptedFields {
		if strings.EqualFold(field.Purpose, input.FocusedPurpose) {
			return fmt.Errorf("focused application state purpose %q is already accepted", input.FocusedPurpose)
		}
	}
	return nil
}

func (input ApplicationRecordFieldLeafInput) validate() error {
	if err := input.Authority.Validate(); err != nil {
		return err
	}
	if err := validateApplicationServiceStatePurpose("record parent", input.ParentPurpose); err != nil {
		return err
	}
	if input.AcceptedRecordFields == nil {
		return fmt.Errorf("application record field accepted set must be non-nil")
	}
	if len(input.AcceptedRecordFields) > maxApplicationServiceStateInterfaceFields {
		return fmt.Errorf(
			"application record field accepted set exceeds %d fields",
			maxApplicationServiceStateInterfaceFields,
		)
	}
	seenPurposes := make(map[string]struct{}, len(input.AcceptedRecordFields))
	for index, field := range input.AcceptedRecordFields {
		expectedName, err := CodeOwnedApplicationServiceRecordFieldName(index + 1)
		if err != nil {
			return err
		}
		if field.Name != expectedName {
			return fmt.Errorf(
				"accepted application record field %d name %q differs from code-owned name %q",
				index+1, field.Name, expectedName,
			)
		}
		if err := validateApplicationServiceStatePurpose("record field", field.Purpose); err != nil {
			return fmt.Errorf("accepted application record field %d: %w", index+1, err)
		}
		purposeKey := strings.ToLower(field.Purpose)
		if _, duplicate := seenPurposes[purposeKey]; duplicate {
			return fmt.Errorf("accepted application record field %d repeats purpose %q", index+1, field.Purpose)
		}
		seenPurposes[purposeKey] = struct{}{}
		if err := validateApplicationServiceStateRecordField(field); err != nil {
			return fmt.Errorf("accepted application record field %d: %w", index+1, err)
		}
	}
	return nil
}

func (input ApplicationRecordFieldKindInput) validate() error {
	if err := (ApplicationRecordFieldLeafInput{
		Authority: input.Authority, ParentPurpose: input.ParentPurpose,
		AcceptedRecordFields: input.AcceptedRecordFields,
	}).validate(); err != nil {
		return err
	}
	if err := validateApplicationServiceStatePurpose("focused record field", input.FocusedPurpose); err != nil {
		return err
	}
	for _, field := range input.AcceptedRecordFields {
		if strings.EqualFold(field.Purpose, input.FocusedPurpose) {
			return fmt.Errorf("focused application record purpose %q is already accepted", input.FocusedPurpose)
		}
	}
	return nil
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
		"Return exactly the requested raw leaf and nothing else.",
		label + ":\n" + string(authority),
	}, "\n\n")
	if len(prompt) > maxPortablePayloadBytes {
		return "", fmt.Errorf("application service state leaf prompt exceeds %d bytes", maxPortablePayloadBytes)
	}
	return prompt, nil
}

func decodeApplicationServiceCoverageLeaf(
	label string,
	raw string,
	remains string,
	complete string,
) (string, error) {
	leaf, err := decodeRawSemanticLeaf(label, raw, 32, false)
	if err != nil {
		return "", err
	}
	if leaf != remains && leaf != complete {
		return "", fmt.Errorf("%s value %q is not registered", label, leaf)
	}
	return leaf, nil
}

func decodeUnacceptedApplicationServicePurpose(
	label string,
	raw string,
	alreadyAccepted func(string) bool,
) (string, error) {
	leaf, err := decodeRawSemanticLeaf(
		label, raw, MaxApplicationServiceStatePurposeBytes, false,
	)
	if err != nil {
		return "", err
	}
	if err := validateApplicationServiceStatePurpose(label, leaf); err != nil {
		return "", err
	}
	switch leaf {
	case ApplicationStateFieldRemains,
		ApplicationNoUncoveredStateField,
		ApplicationRecordFieldRemains,
		ApplicationNoUncoveredRecordField:
		return "", fmt.Errorf("%s %q is a reserved coverage value", label, leaf)
	}
	if alreadyAccepted(leaf) {
		return "", fmt.Errorf("%s %q is already accepted", label, leaf)
	}
	return leaf, nil
}
