package assemblyline

import "fmt"

func BuildApplicationRecordFieldCoveragePrompt(
	input ApplicationRecordFieldLeafInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic relation: does the focused record-list field require any scalar record member that is not semantically covered by the accepted record fields?",
		"Return RECORD_FIELD_REMAINS when one or more scalar members remain. Return NO_UNCOVERED_RECORD_FIELD when the accepted members are minimally sufficient.",
		"APPLICATION_RECORD_FIELD_AUTHORITY", input,
	)
}

func BuildApplicationRecordFieldNamePrompt(
	input ApplicationRecordFieldLeafInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Return one canonical lowercase snake-case name for the earliest necessary scalar member of the focused record-list field not semantically covered by the accepted record fields.",
		"Return only that one raw identifier. Do not return its kind, another member, JSON, quotes, a label, Markdown, or commentary.",
		"APPLICATION_RECORD_FIELD_AUTHORITY", input,
	)
}

func BuildApplicationRecordFieldKindPrompt(
	input ApplicationRecordFieldKindInput,
) (string, error) {
	return buildApplicationServiceStateLeafPrompt(
		input.validate,
		"Answer one semantic question: what registered scalar data kind must the focused record member use to satisfy the directly related behavior authority?",
		"Return exactly one raw registered scalar kind: string, integer, number, or boolean. Return no JSON, quotes, label, Markdown, or commentary.",
		"APPLICATION_RECORD_FIELD_KIND_AUTHORITY", input,
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

func DecodeApplicationRecordFieldNameLeaf(
	input ApplicationRecordFieldLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	return decodeUnacceptedApplicationServiceFieldName(
		"application record field name", raw,
		func(name string) bool {
			for _, field := range input.AcceptedRecordFields {
				if field.Name == name {
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
