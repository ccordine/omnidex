package assemblyline

import "fmt"

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
