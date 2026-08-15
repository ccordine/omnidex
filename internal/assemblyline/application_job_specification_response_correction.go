package assemblyline

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

func applicationJobSpecificationResponseCorrectionSchema(target string) (map[string]any, error) {
	_, _, definition, err := parseApplicationJobSpecificationCorrectionTarget(target)
	if err != nil {
		return nil, err
	}
	return objectSchema([]string{target}, map[string]any{target: definition}), nil
}

func buildApplicationJobSpecificationResponseCorrectionPrompt(
	input ResponseCorrectionInput,
) (string, error) {
	var authority ApplicationJobSpecificationInput
	if err := decodePortablePayload(input.Original.Payload, &authority); err != nil {
		return "", err
	}
	if err := validateApplicationJobSpecificationInput(authority); err != nil {
		return "", err
	}
	projection := struct {
		UserAuthority applicationJobSpecificationAuthority `json:"user_authority"`
		TargetLeaf    string                               `json:"target_leaf"`
	}{
		UserAuthority: projectApplicationJobSpecificationAuthority(authority),
		TargetLeaf:    input.TargetField,
	}
	raw, err := json.Marshal(projection)
	if err != nil {
		return "", fmt.Errorf("encode application specification correction authority: %w", err)
	}
	return strings.Join([]string{
		"Replace exactly target_leaf in the retained response. The retained response and every accepted leaf remain code-owned and unavailable.",
		"Return only the one-field JSON replacement. Derive the minimum sufficient replacement from user_authority and resolve exactly this validation failure:\n" + input.ValidationFailure,
		"APPLICATION_JOB_SPECIFICATION_LEAF_CORRECTION_AUTHORITY_JSON:\n" + string(raw),
	}, "\n\n"), nil
}

func applyApplicationJobSpecificationResponseCorrection(
	original PortableJob,
	retainedCandidate string,
	mergePatch string,
	target string,
) (string, error) {
	if _, err := applicationJobSpecificationResponseCorrectionSchema(target); err != nil {
		return "", err
	}
	var authority ApplicationJobSpecificationInput
	if err := decodePortablePayload(original.Payload, &authority); err != nil {
		return "", err
	}
	retained, err := DecodeApplicationJobSpecification(authority, retainedCandidate)
	if err != nil {
		return "", fmt.Errorf("decode retained application job specification: %w", err)
	}
	defect := FirstApplicationJobSpecificationDefect(retained)
	currentTarget, correctable := defect.CorrectionTarget()
	if defect == nil || !correctable {
		return "", fmt.Errorf("application job specification has no response-correctable leaf")
	}
	if currentTarget != target {
		return "", fmt.Errorf(
			"application job specification correction target %q does not own current defect %q",
			target, currentTarget,
		)
	}
	patch, err := decodeJSONObject(mergePatch, "application job specification response patch")
	if err != nil {
		return "", err
	}
	if len(patch) != 1 {
		return "", fmt.Errorf("application job specification response patch must contain exactly one leaf")
	}
	replacement, exists := patch[target]
	if !exists {
		return "", fmt.Errorf("application job specification response patch must replace %q", target)
	}
	value, ok := replacement.(string)
	if !ok {
		return "", fmt.Errorf("application job specification response patch %q must be a string", target)
	}
	field, index, _, err := parseApplicationJobSpecificationCorrectionTarget(target)
	if err != nil {
		return "", err
	}
	if err := applyApplicationJobSpecificationLeaf(&retained, field, index, value); err != nil {
		return "", err
	}
	raw, err := json.Marshal(retained)
	if err != nil {
		return "", fmt.Errorf("encode corrected application job specification: %w", err)
	}
	return string(raw), nil
}

func applyApplicationJobSpecificationLeaf(
	specification *ApplicationJobSpecification,
	field ApplicationJobSpecificationField,
	index int,
	value string,
) error {
	switch field {
	case ApplicationJobSpecificationObjectiveField:
		if specification.Objective == value {
			return fmt.Errorf("application job specification correction is a no-op")
		}
		specification.Objective = value
	case ApplicationJobSpecificationRequiredBehaviorsField:
		if specification.RequiredBehaviors[index] == value {
			return fmt.Errorf("application job specification correction is a no-op")
		}
		specification.RequiredBehaviors[index] = value
	case ApplicationJobSpecificationAcceptanceCriteriaField:
		if specification.AcceptanceCriteria[index] == value {
			return fmt.Errorf("application job specification correction is a no-op")
		}
		specification.AcceptanceCriteria[index] = value
	default:
		return fmt.Errorf("application job specification correction field %q is unsupported", field)
	}
	return nil
}

func parseApplicationJobSpecificationCorrectionTarget(
	target string,
) (ApplicationJobSpecificationField, int, map[string]any, error) {
	if target == "objective" {
		return ApplicationJobSpecificationObjectiveField, -1,
			applicationJobSpecificationLineSchema(maxApplicationObjectiveRunes), nil
	}
	for _, candidate := range []struct {
		field   ApplicationJobSpecificationField
		maximum int
	}{
		{ApplicationJobSpecificationRequiredBehaviorsField, maxApplicationBehaviorRunes},
		{ApplicationJobSpecificationAcceptanceCriteriaField, maxApplicationCriterionRunes},
	} {
		prefix := string(candidate.field) + "_"
		if !strings.HasPrefix(target, prefix) {
			continue
		}
		ordinal, err := strconv.Atoi(strings.TrimPrefix(target, prefix))
		if err != nil || ordinal < 1 || ordinal > 4 || fmt.Sprintf("%03d", ordinal) != strings.TrimPrefix(target, prefix) {
			break
		}
		return candidate.field, ordinal - 1,
			applicationJobSpecificationLineSchema(candidate.maximum), nil
	}
	return "", 0, nil, fmt.Errorf(
		"application job specification correction target %q is unsupported", target,
	)
}
