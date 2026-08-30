package assemblyline

import "fmt"

func BuildApplicationRecordFieldKindPrompt(
	input ApplicationRecordFieldKindInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic question: what registered scalar data kind must the focused record member use to fulfill its exact purpose?",
		"Return exactly one raw registered scalar kind: string, integer, number, or boolean. Return no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION_RECORD_FIELD_KIND_AUTHORITY",
		applicationRecordFieldKindProjection{
			Authority:      input.Authority,
			ParentPurpose:  input.ParentPurpose,
			FocusedPurpose: input.FocusedPurpose,
		},
	)
}

func DecodeApplicationRecordFieldKindLeaf(
	input ApplicationRecordFieldKindInput,
	raw string,
) (ApplicationServiceStateFieldKind, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf("application record field kind", raw, 32, false)
	if err != nil {
		return "", err
	}
	kind := ApplicationServiceStateFieldKind(leaf)
	switch kind {
	case ApplicationServiceStateString, ApplicationServiceStateInteger,
		ApplicationServiceStateNumber, ApplicationServiceStateBoolean:
		return kind, nil
	default:
		return "", fmt.Errorf("application record field kind %q is not a registered scalar kind", leaf)
	}
}
