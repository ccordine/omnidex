package assemblyline

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

func BuildApplicationRequirementCandidateResultPresencePrompt(
	input ApplicationRequirementCandidateResultPresenceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var question string
	var presentDescription string
	var absentDescription string
	switch input.Dimension {
	case ApplicationRequirementDerivedValueDimension:
		question = "Does the candidate assert a derived runtime value? A value is derived when it is selected, ordered, transformed, read, extracted, decoded, hashed, grouped, aggregated, measured, calculated, or decided from inputs. A named result-bearing operation over its governed object qualifies even when phrased as an action. An action, control, state transition, event, message, artifact creation or availability, unchanged supplied data, trigger condition, or qualitative adjective alone does not create a derived value."
		presentDescription = "The candidate asserts a derived runtime value."
		absentDescription = "The candidate does not assert a derived runtime value."
	case ApplicationRequirementDeterminingRelationDimension:
		question = "The candidate asserts a derived runtime value. Does it state an independently computable determining relation for that value? A relation exists when the candidate names one exact rule with its necessary input or condition, a family of result-bearing operations over governed inputs, or one named intrinsic or mechanically observable property of a governed object. The object and named intrinsic property suffice without restating a measurement procedure. An operation-family name and governed inputs suffice because the invoked member and operand values are runtime inputs. Named dimensions, lengths, counts, orderings, digests, comparisons, selections, aggregations, and existing per-item grouping keys are rules. Merely calling an output calculated, computed, evaluated, generated, selected, correct, best, useful, appropriate, high-quality, or otherwise desirable supplies no rule."
		presentDescription = "The candidate states an independently computable determining relation for the derived value."
		absentDescription = "The candidate does not state an independently computable determining relation for the derived value."
	default:
		return "", fmt.Errorf("application requirement candidate result dimension %q is not registered", input.Dimension)
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
	authoritySHA256, err := applicationRequirementCandidateResultPresenceAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCandidateResultPresenceResult{
		Schema:          ApplicationRequirementCandidateResultPresenceSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Presence:        presence,
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
		return "The candidate states an independently computable determining relation for the derived value.",
			"The candidate does not state an independently computable determining relation for the derived value.", nil
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

func applicationRequirementCandidateResultPresenceAuthoritySHA256(
	input ApplicationRequirementCandidateResultPresenceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application requirement candidate result presence authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}
