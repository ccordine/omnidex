package assemblyline

import "fmt"

func BuildApplicationRecordFieldCoveragePrompt(
	input ApplicationRecordFieldLeafInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic question: does the focused record-list value require another scalar member not covered by the accepted semantic members?",
		"Return RECORD_FIELD_REMAINS when one or more scalar members remain. Return NO_UNCOVERED_RECORD_FIELD when the accepted members are minimally sufficient.",
		"APPLICATION_RECORD_FIELD_AUTHORITY",
		applicationRecordFieldLeafProjection{
			Authority:            input.Authority,
			ParentPurpose:        input.ParentPurpose,
			AcceptedRecordFields: projectApplicationRecordFields(input.AcceptedRecordFields),
		},
	)
}

func BuildApplicationRecordFieldPurposePrompt(
	input ApplicationRecordFieldLeafInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"State one specific semantic responsibility of a necessary scalar member not covered in the focused record-list value.",
		"Return only one concise raw purpose sentence. Do not return an identifier, data kind, another member, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION_RECORD_FIELD_AUTHORITY",
		applicationRecordFieldLeafProjection{
			Authority:            input.Authority,
			ParentPurpose:        input.ParentPurpose,
			AcceptedRecordFields: projectApplicationRecordFields(input.AcceptedRecordFields),
		},
	)
}

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

func DecodeApplicationRecordFieldCoverageLeaf(
	input ApplicationRecordFieldLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return decodeApplicationServiceCoverageLeaf(
		"application record field coverage", raw,
		ApplicationRecordFieldRemains, ApplicationNoUncoveredRecordField,
	)
}

func DecodeApplicationRecordFieldPurposeLeaf(
	input ApplicationRecordFieldLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return decodeUnacceptedApplicationServicePurpose(
		"application record field purpose", raw,
		func(purpose string) bool {
			for _, field := range input.AcceptedRecordFields {
				if equalApplicationServicePurpose(field.Purpose, purpose) {
					return true
				}
			}
			return false
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
