package assemblyline

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	WorkApplicationStateFieldCoverage         WorkKind = "application_state_field_coverage"
	WorkApplicationStateFieldName             WorkKind = "application_state_field_name"
	WorkApplicationStateFieldKind             WorkKind = "application_state_field_kind"
	WorkApplicationRecordFieldCoverage        WorkKind = "application_record_field_coverage"
	WorkApplicationRecordFieldName            WorkKind = "application_record_field_name"
	WorkApplicationRecordFieldKind            WorkKind = "application_record_field_kind"
	MaxApplicationServiceStateInterfaceFields          = maxApplicationServiceStateInterfaceFields

	ApplicationStateFieldRemains      = "STATE_FIELD_REMAINS"
	ApplicationNoUncoveredStateField  = "NO_UNCOVERED_STATE_FIELD"
	ApplicationRecordFieldRemains     = "RECORD_FIELD_REMAINS"
	ApplicationNoUncoveredRecordField = "NO_UNCOVERED_RECORD_FIELD"
)

type ApplicationStateFieldLeafInput struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	AcceptedFields []ApplicationServiceStateField        `json:"accepted_fields"`
}

type ApplicationStateFieldKindInput struct {
	Authority      ApplicationServiceStateInterfaceInput `json:"authority"`
	AcceptedFields []ApplicationServiceStateField        `json:"accepted_fields"`
	FocusedName    string                                `json:"focused_name"`
}

type ApplicationRecordFieldLeafInput struct {
	Authority            ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentName           string                                `json:"parent_name"`
	AcceptedRecordFields []ApplicationServiceStateRecordField  `json:"accepted_record_fields"`
}

type ApplicationRecordFieldKindInput struct {
	Authority            ApplicationServiceStateInterfaceInput `json:"authority"`
	ParentName           string                                `json:"parent_name"`
	AcceptedRecordFields []ApplicationServiceStateRecordField  `json:"accepted_record_fields"`
	FocusedName          string                                `json:"focused_name"`
}

func NewApplicationStateFieldCoverageJob(
	input ApplicationStateFieldLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationStateFieldCoverage, input, input.validate,
	)
}

func NewApplicationStateFieldNameJob(
	input ApplicationStateFieldLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationStateFieldName, input, input.validate)
}

func NewApplicationStateFieldKindJob(
	input ApplicationStateFieldKindInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationStateFieldKind, input, input.validate)
}

func NewApplicationRecordFieldCoverageJob(
	input ApplicationRecordFieldLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRecordFieldCoverage, input, input.validate,
	)
}

func NewApplicationRecordFieldNameJob(
	input ApplicationRecordFieldLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationRecordFieldName, input, input.validate)
}

func NewApplicationRecordFieldKindJob(
	input ApplicationRecordFieldKindInput,
) (PortableJob, error) {
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
	seen := make(map[string]struct{}, len(input.AcceptedFields))
	for index, field := range input.AcceptedFields {
		if err := validateApplicationServiceStateFieldName(field.Name); err != nil {
			return fmt.Errorf("accepted application state field %d: %w", index, err)
		}
		if _, duplicate := seen[field.Name]; duplicate {
			return fmt.Errorf("accepted application state field %d repeats %q", index, field.Name)
		}
		seen[field.Name] = struct{}{}
		if err := validateApplicationServiceStateField(field); err != nil {
			return fmt.Errorf("accepted application state field %q: %w", field.Name, err)
		}
	}
	return nil
}

func (input ApplicationStateFieldKindInput) validate() error {
	if err := (ApplicationStateFieldLeafInput{
		Authority: input.Authority, AcceptedFields: input.AcceptedFields,
	}).validate(); err != nil {
		return err
	}
	if err := validateApplicationServiceStateFieldName(input.FocusedName); err != nil {
		return fmt.Errorf("focused application state field: %w", err)
	}
	for _, field := range input.AcceptedFields {
		if field.Name == input.FocusedName {
			return fmt.Errorf("focused application state field %q is already accepted", input.FocusedName)
		}
	}
	return nil
}

func (input ApplicationRecordFieldLeafInput) validate() error {
	if err := input.Authority.Validate(); err != nil {
		return err
	}
	if err := validateApplicationServiceStateFieldName(input.ParentName); err != nil {
		return fmt.Errorf("application record parent: %w", err)
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
	seen := make(map[string]struct{}, len(input.AcceptedRecordFields))
	for index, field := range input.AcceptedRecordFields {
		if err := validateApplicationServiceStateRecordField(field); err != nil {
			return fmt.Errorf("accepted application record field %d: %w", index, err)
		}
		if _, duplicate := seen[field.Name]; duplicate {
			return fmt.Errorf("accepted application record field %d repeats %q", index, field.Name)
		}
		seen[field.Name] = struct{}{}
	}
	return nil
}

func (input ApplicationRecordFieldKindInput) validate() error {
	if err := (ApplicationRecordFieldLeafInput{
		Authority: input.Authority, ParentName: input.ParentName,
		AcceptedRecordFields: input.AcceptedRecordFields,
	}).validate(); err != nil {
		return err
	}
	if err := validateApplicationServiceStateFieldName(input.FocusedName); err != nil {
		return fmt.Errorf("focused application record field: %w", err)
	}
	for _, field := range input.AcceptedRecordFields {
		if field.Name == input.FocusedName {
			return fmt.Errorf("focused application record field %q is already accepted", input.FocusedName)
		}
	}
	return nil
}

func validateApplicationServiceStateRecordField(
	field ApplicationServiceStateRecordField,
) error {
	if err := validateApplicationServiceStateFieldName(field.Name); err != nil {
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

func buildApplicationServiceStateLeafPrompt[T any](
	validate func() error,
	question string,
	response string,
	label string,
	input T,
) (string, error) {
	if err := validate(); err != nil {
		return "", err
	}
	authority, err := json.Marshal(input)
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

func decodeUnacceptedApplicationServiceFieldName(
	label string,
	raw string,
	alreadyAccepted func(string) bool,
) (string, error) {
	leaf, err := decodeRawSemanticLeaf(label, raw, 48, false)
	if err != nil {
		return "", err
	}
	if err := validateApplicationServiceStateFieldName(leaf); err != nil {
		return "", err
	}
	if alreadyAccepted(leaf) {
		return "", fmt.Errorf("%s %q is already accepted", label, leaf)
	}
	return leaf, nil
}
