package assemblyline

import "fmt"

func BuildApplicationStateFieldCoveragePrompt(
	input ApplicationStateFieldLeafInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic question: does the directly related behavior authority require another durable root state value not covered by the accepted semantic values?",
		"Return STATE_FIELD_REMAINS when one or more root values remain. Return NO_UNCOVERED_STATE_FIELD when the accepted values form the minimal sufficient durable interface.",
		"APPLICATION_STATE_FIELD_AUTHORITY",
		applicationStateFieldLeafProjection{
			Authority:      input.Authority,
			AcceptedFields: projectApplicationStateFields(input.AcceptedFields),
		},
	)
}

func BuildApplicationStateFieldPurposePrompt(
	input ApplicationStateFieldLeafInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"State one specific semantic responsibility of a necessary durable root value not covered by the accepted semantic values.",
		"Return only one concise raw purpose sentence. Do not return an identifier, data kind, record members, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION_STATE_FIELD_AUTHORITY",
		applicationStateFieldLeafProjection{
			Authority:      input.Authority,
			AcceptedFields: projectApplicationStateFields(input.AcceptedFields),
		},
	)
}

func BuildApplicationStateFieldKindPrompt(
	input ApplicationStateFieldKindInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic question: what registered data kind must the focused durable root value use to fulfill its exact purpose?",
		"Return exactly one raw registered kind: string, integer, number, boolean, string_list, integer_list, number_list, boolean_list, or record_list. Return no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION_STATE_FIELD_KIND_AUTHORITY",
		applicationStateFieldKindProjection{
			Authority:      input.Authority,
			FocusedPurpose: input.FocusedPurpose,
		},
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

func DecodeApplicationStateFieldPurposeLeaf(
	input ApplicationStateFieldLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return decodeUnacceptedApplicationServicePurpose(
		"application state field purpose", raw,
		func(purpose string) bool {
			for _, field := range input.AcceptedFields {
				if equalApplicationServicePurpose(field.Purpose, purpose) {
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
