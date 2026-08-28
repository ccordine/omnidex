package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkContextSearchTermCoverage WorkKind = "context_search_term_coverage"
	WorkContextSearchTerm         WorkKind = "context_search_term"

	ContextTermRemains     = "CONTEXT_TERM_REMAINS"
	ContextNoUncoveredTerm = "NO_UNCOVERED_CONTEXT_TERM"
)

// ContextSearchTermLeafInput carries the exact current instruction and the
// code-retained retrieval concepts. Scope remains code-only routing authority
// and is deliberately absent from the rendered model context.
type ContextSearchTermLeafInput struct {
	ExactInstruction string       `json:"exact_instruction"`
	Scope            ContextScope `json:"scope,omitempty"`
	AcceptedTerms    []string     `json:"accepted_terms"`
}

func NewContextSearchTermCoverageJob(
	input ContextSearchTermLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkContextSearchTermCoverage, input, input.validate,
	)
}

func NewContextSearchTermJob(
	input ContextSearchTermLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkContextSearchTerm, input, input.validate,
	)
}

func (input ContextSearchTermLeafInput) validate() error {
	base := ContextSearchTermsInput{
		ExactInstruction: input.ExactInstruction,
		Scope:            input.Scope,
	}
	if err := base.validate(); err != nil {
		return err
	}
	if input.AcceptedTerms == nil {
		return fmt.Errorf("context search term leaf requires a non-nil accepted set")
	}
	return (ContextSearchTermsDecision{
		Schema: ContextSearchTermsSchemaV1,
		Terms:  append([]string{}, input.AcceptedTerms...),
	}).ValidateFor(base)
}

func BuildContextSearchTermCoveragePrompt(
	input ContextSearchTermLeafInput,
) (string, error) {
	authority, err := renderContextSearchTermAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Answer one semantic coverage relation: does the exact current instruction contain one unresolved prior referent or action that is not semantically covered by the accepted retrieval concepts?",
		"A self-contained instruction, including a greeting, has no unresolved retrieval concept. A useful concept names only a prior referent or action needed to interpret an anaphoric or elliptical instruction; it is not a path, operation, query plan, or answer.",
		"Return CONTEXT_TERM_REMAINS when one retrieval concept remains uncovered. Return NO_UNCOVERED_CONTEXT_TERM when none remains.",
		"Return exactly that registered raw value and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"CONTEXT_SEARCH_TERM_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeContextSearchTermCoverageLeaf(
	input ContextSearchTermLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"context search term coverage", raw, 32, false,
	)
	if err != nil {
		return "", err
	}
	switch leaf {
	case ContextTermRemains, ContextNoUncoveredTerm:
		return leaf, nil
	default:
		return "", fmt.Errorf(
			"context search term coverage value %q is not registered", leaf,
		)
	}
}

func BuildContextSearchTermPrompt(
	input ContextSearchTermLeafInput,
) (string, error) {
	authority, err := renderContextSearchTermAuthority(input)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		"Return exactly one concise retrieval concept for an unresolved prior referent or action in the exact current instruction that is not semantically covered by the accepted concepts.",
		"Name only that referent or prior action. Do not return a path, operation, query plan, answer, or a second concept.",
		"Return only the concept as one raw line. Do not return JSON, quotes, a label, Markdown, or commentary.",
		"CONTEXT_SEARCH_TERM_AUTHORITY:\n" + authority,
	}, "\n\n"), nil
}

func DecodeContextSearchTermLeaf(
	input ContextSearchTermLeafInput,
	raw string,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	leaf, err := decodeRawSemanticLeaf(
		"context search term", raw, MaxContextSearchTermBytes, false,
	)
	if err != nil {
		return "", err
	}
	if err := validateContextSearchTerm(leaf); err != nil {
		return "", err
	}
	if err := ValidatePathFreeModelContext("context search term", leaf); err != nil {
		return "", err
	}
	for _, accepted := range input.AcceptedTerms {
		if strings.EqualFold(leaf, accepted) {
			return "", fmt.Errorf("context search term duplicates an accepted term")
		}
	}
	return leaf, nil
}

func AssembleContextSearchTermsDecision(
	input ContextSearchTermsInput,
	terms []string,
) (ContextSearchTermsDecision, error) {
	decision := ContextSearchTermsDecision{
		Schema: ContextSearchTermsSchemaV1,
		Terms:  append([]string{}, terms...),
	}
	if err := decision.ValidateFor(input); err != nil {
		return ContextSearchTermsDecision{}, err
	}
	return decision, nil
}

func renderContextSearchTermAuthority(
	input ContextSearchTermLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var authority strings.Builder
	fmt.Fprintf(&authority, "EXACT CURRENT INSTRUCTION:\n%s\n", input.ExactInstruction)
	if len(input.AcceptedTerms) == 0 {
		authority.WriteString("ACCEPTED RETRIEVAL CONCEPTS:\n(none)\n")
	} else {
		for index, term := range input.AcceptedTerms {
			fmt.Fprintf(&authority, "ACCEPTED RETRIEVAL CONCEPT %d:\n%s\n", index+1, term)
		}
	}
	return strings.TrimSuffix(authority.String(), "\n"), nil
}
