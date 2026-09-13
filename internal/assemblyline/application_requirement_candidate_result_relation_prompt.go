package assemblyline

import (
	"fmt"
)

func BuildApplicationRequirementCandidateResultPresencePrompt(
	input ApplicationRequirementCandidateResultPresenceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var question string
	switch input.Dimension {
	case ApplicationRequirementDerivedValueDimension:
		question = "Does the candidate assert a derived runtime value? A value is derived when it is selected, ordered, transformed, read, extracted, decoded, hashed, grouped, aggregated, measured, calculated, or decided from inputs. A named result-bearing operation over its governed object qualifies even when phrased as an action. An action, control, state transition, event, message, artifact creation or availability, unchanged supplied data, trigger condition, or qualitative adjective alone does not create a derived value."
	case ApplicationRequirementDeterminingRelationDimension:
		question = "The candidate requires a derived value. Does it name what to compute or measure? A named operation, operation family, or intrinsic measurable property specifies the rule; the concrete operands can be runtime inputs. A desired assessment without a named rule or measurable property does not."
	default:
		return "", fmt.Errorf("application requirement candidate result dimension %q is not registered", input.Dimension)
	}
	presentDescription, absentDescription, err := applicationRequirementCandidateResultPresenceDescriptions(input.Dimension)
	if err != nil {
		return "", err
	}
	choices, err := applicationRequirementCandidateResultPresenceOpaqueChoices(
		presentDescription,
		absentDescription,
	)
	if err != nil {
		return "", err
	}
	return RenderOpaqueModelChoiceQuestion(
		question+" Inspect only this candidate.",
		[]string{"Requirement candidate:\n" + input.Candidate},
		choices,
	)
}

func DecodeApplicationRequirementCandidateResultPresenceResult(
	input ApplicationRequirementCandidateResultPresenceInput,
	raw string,
) (ApplicationRequirementCandidateResultPresenceResult, error) {
	var zero ApplicationRequirementCandidateResultPresenceResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	presentDescription, absentDescription, err := applicationRequirementCandidateResultPresenceDescriptions(input.Dimension)
	if err != nil {
		return zero, err
	}
	choices, err := applicationRequirementCandidateResultPresenceOpaqueChoices(
		presentDescription,
		absentDescription,
	)
	if err != nil {
		return zero, err
	}
	leaf, err := DecodeOpaqueModelChoice(raw, choices)
	if err != nil {
		return zero, err
	}
	return applicationRequirementCandidateResultPresenceResult(
		input,
		ApplicationRequirementCandidateResultPresence(leaf),
	)
}

func applicationRequirementCandidateResultPresenceResult(
	input ApplicationRequirementCandidateResultPresenceInput,
	presence ApplicationRequirementCandidateResultPresence,
) (ApplicationRequirementCandidateResultPresenceResult, error) {
	var zero ApplicationRequirementCandidateResultPresenceResult
	result := ApplicationRequirementCandidateResultPresenceResult{
		Schema:   ApplicationRequirementCandidateResultPresenceSchemaV1,
		Presence: presence,
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
}

func applicationRequirementCandidateResultPresenceDescriptions(
	dimension ApplicationRequirementCandidateResultDimension,
) (string, string, error) {
	switch dimension {
	case ApplicationRequirementDerivedValueDimension:
		return "The candidate asserts a derived runtime value.",
			"The candidate does not assert a derived runtime value.", nil
	case ApplicationRequirementDeterminingRelationDimension:
		return "The candidate names the computation, operation family, or measurable property.",
			"The candidate leaves the computation or judgment rule unspecified.", nil
	default:
		return "", "", fmt.Errorf(
			"application requirement candidate result dimension %q is not registered",
			dimension,
		)
	}
}

func applicationRequirementCandidateResultPresenceOpaqueChoices(
	presentDescription string,
	absentDescription string,
) ([]OpaqueModelChoice, error) {
	present, err := NewOpaqueModelChoice(
		presentDescription,
		string(ApplicationRequirementCandidateResultPresent),
	)
	if err != nil {
		return nil, err
	}
	absent, err := NewOpaqueModelChoice(
		absentDescription,
		string(ApplicationRequirementCandidateResultAbsent),
	)
	if err != nil {
		return nil, err
	}
	return []OpaqueModelChoice{present, absent}, nil
}
