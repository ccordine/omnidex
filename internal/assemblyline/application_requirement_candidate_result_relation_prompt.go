package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

func BuildApplicationRequirementCandidateResultPresencePrompt(
	input ApplicationRequirementCandidateResultPresenceInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var question []string
	switch input.Dimension {
	case ApplicationRequirementDerivedValueDimension:
		question = []string{
			"Answer one semantic presence question about the exact candidate: does it assert a derived runtime value?",
			"A derived value is selected, ordered, transformed, read, extracted, decoded, hashed, grouped, aggregated, measured, calculated, or decided from inputs. A named result-bearing operation over its governed object is PRESENT even when phrased as an action.",
			"Return ABSENT when the candidate asserts only an action, control, state transition, event, message, artifact creation or availability, or unchanged supplied data. A trigger condition and qualitative adjective do not create a derived value.",
			"FINAL QUESTION:\nIs a derived runtime value PRESENT or ABSENT? Return only PRESENT or ABSENT.",
		}
	case ApplicationRequirementDeterminingRelationDimension:
		question = []string{
			"The exact candidate asserts a derived runtime value. Answer one semantic presence question: does it state an independently computable determining relation for that value?",
			"Return PRESENT only when the candidate names the necessary input or condition and determining rule. A named result-bearing operation uses its governed object as input and the operation as rule. Named orderings, digests, comparisons, selections, aggregations, and existing per-item grouping keys are rules; equal key values determine groups.",
			"An actor-supplied expression, formula, or operation, or an actor-performed calculation, supplies runtime rule and operands. Passively calling an unspecified output calculated, computed, evaluated, generated, selected, correct, best, useful, or appropriate supplies no rule.",
			"FINAL QUESTION:\nIs the independently computable determining relation PRESENT or ABSENT? Return only PRESENT or ABSENT.",
		}
	default:
		return "", fmt.Errorf("application requirement candidate result dimension %q is not registered", input.Dimension)
	}
	return strings.Join([]string{
		question[0], question[1], question[2],
		"Inspect only the exact candidate. Return only the raw registered presence with no JSON, label, Markdown, or explanation.",
		"EXACT REQUIREMENT CANDIDATE:\n" + input.Candidate,
		question[3],
	}, "\n\n"), nil
}

func DecodeApplicationRequirementCandidateResultPresenceResult(
	input ApplicationRequirementCandidateResultPresenceInput,
	raw string,
) (ApplicationRequirementCandidateResultPresenceResult, error) {
	var zero ApplicationRequirementCandidateResultPresenceResult
	if err := input.validate(); err != nil {
		return zero, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"application requirement candidate result presence",
		raw,
		maximumStringBytes(
			string(ApplicationRequirementCandidateResultPresent),
			string(ApplicationRequirementCandidateResultAbsent),
		),
		false,
	)
	if err != nil {
		return zero, err
	}
	authoritySHA256, err := applicationRequirementCandidateResultPresenceAuthoritySHA256(input)
	if err != nil {
		return zero, err
	}
	result := ApplicationRequirementCandidateResultPresenceResult{
		Schema:          ApplicationRequirementCandidateResultPresenceSchemaV1,
		AuthoritySHA256: authoritySHA256,
		Presence:        ApplicationRequirementCandidateResultPresence(leaf),
	}
	if err := result.ValidateFor(input); err != nil {
		return zero, err
	}
	return result, nil
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
