package assemblyline

import "fmt"

func BuildApplicationStateFieldCoveragePrompt(
	input ApplicationStateFieldLeafInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic relation: does the directly related behavior authority require any durable root state field that is not semantically covered by the accepted fields?",
		"Return STATE_FIELD_REMAINS when one or more root fields remain. Return NO_UNCOVERED_STATE_FIELD when the accepted fields form the minimal sufficient durable interface.",
		"APPLICATION_STATE_FIELD_AUTHORITY", input,
	)
}

func BuildApplicationStateFieldNamePrompt(
	input ApplicationStateFieldLeafInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Return one canonical lowercase snake-case name for the earliest necessary durable root state field not semantically covered by the accepted fields.",
		"Return only that one raw identifier. Do not return its kind, record members, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION_STATE_FIELD_AUTHORITY", input,
	)
}

func BuildApplicationStateFieldKindPrompt(
	input ApplicationStateFieldKindInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic question: what registered data kind must the focused durable root state field use to satisfy the directly related behavior authority?",
		"Return exactly one raw registered kind: string, integer, number, boolean, string_list, integer_list, number_list, boolean_list, or record_list. Return no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION_STATE_FIELD_KIND_AUTHORITY", input,
	)
}

func DecodeApplicationStateFieldCoverageLeaf(
	input ApplicationStateFieldLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return decodeApplicationServiceCoverageLeaf(
		"application state field coverage", raw,
		ApplicationStateFieldRemains, ApplicationNoUncoveredStateField,
	)
}

func DecodeApplicationStateFieldNameLeaf(
	input ApplicationStateFieldLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return decodeUnacceptedApplicationServiceFieldName(
		"application state field name", raw,
		func(name string) bool {
			for _, field := range input.AcceptedFields {
				if field.Name == name {
					return true
				}
			}
			return false
		},
	)
}

func DecodeApplicationStateFieldKindLeaf(
	input ApplicationStateFieldKindInput,
	raw string,
) (ApplicationServiceStateFieldKind, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf("application state field kind", raw, 32, false)
	if err != nil {
		return "", err
	}
	kind := ApplicationServiceStateFieldKind(leaf)
	switch kind {
	case ApplicationServiceStateString, ApplicationServiceStateInteger,
		ApplicationServiceStateNumber, ApplicationServiceStateBoolean,
		ApplicationServiceStateStringList, ApplicationServiceStateIntegerList,
		ApplicationServiceStateNumberList, ApplicationServiceStateBooleanList,
		ApplicationServiceStateRecordList:
		return kind, nil
	default:
		return "", fmt.Errorf("application state field kind %q is not registered", leaf)
	}
}
