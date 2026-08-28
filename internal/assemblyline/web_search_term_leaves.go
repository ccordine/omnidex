package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkWebSearchTermCoverage WorkKind = "web_search_term_coverage"
	WorkWebSearchTerm         WorkKind = "web_search_term"

	WebQueryTermRemains     WebSearchTermCoverage = "QUERY_TERM_REMAINS"
	WebNoUncoveredQueryTerm WebSearchTermCoverage = "NO_UNCOVERED_QUERY_TERM"
)

type WebSearchTermCoverage string

// WebSearchTermLeafInput retains the terms code has already accepted while
// one model call resolves only coverage or one additional search term.
type WebSearchTermLeafInput struct {
	ExactQuestion    string           `json:"exact_question"`
	Context          ObjectiveContext `json:"objective_context"`
	AttemptedQueries []string         `json:"attempted_queries"`
	AcceptedTerms    []string         `json:"accepted_terms"`
	MaxTerms         int              `json:"max_terms"`
	MaxTermBytes     int              `json:"max_term_bytes"`
}

type WebSearchTermCoverageDecision struct {
	Coverage WebSearchTermCoverage `json:"coverage"`
}

type WebSearchTermDecision struct {
	Term string `json:"term"`
}

func NewWebSearchTermCoverageJob(input WebSearchTermLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkWebSearchTermCoverage, input, input.validate,
	)
}

func NewWebSearchTermJob(input WebSearchTermLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkWebSearchTerm, input, input.validateForTerm)
}

func (input WebSearchTermLeafInput) base() WebSearchTermsInput {
	return WebSearchTermsInput{
		ExactQuestion:    input.ExactQuestion,
		Context:          input.Context,
		AttemptedQueries: append([]string(nil), input.AttemptedQueries...),
		MaxTerms:         input.MaxTerms,
		MaxTermBytes:     input.MaxTermBytes,
	}
}

func (input WebSearchTermLeafInput) validate() error {
	base := input.base()
	if err := base.validate(); err != nil {
		return err
	}
	if input.AcceptedTerms == nil {
		return fmt.Errorf("web search term leaf requires a non-nil accepted set")
	}
	if len(input.AcceptedTerms) > input.MaxTerms {
		return fmt.Errorf(
			"web search term leaf exceeds the %d-term bound", input.MaxTerms,
		)
	}
	return validateAcceptedWebSearchTerms(base, input.AcceptedTerms)
}

func (input WebSearchTermLeafInput) validateForTerm() error {
	if err := input.validate(); err != nil {
		return err
	}
	if len(input.AcceptedTerms) >= input.MaxTerms {
		return fmt.Errorf("web search term bound is exhausted")
	}
	return nil
}

func validateAcceptedWebSearchTerms(input WebSearchTermsInput, terms []string) error {
	seen := make(map[string]struct{}, len(input.AttemptedQueries)+len(terms))
	for _, query := range input.AttemptedQueries {
		seen[strings.ToLower(query)] = struct{}{}
	}
	for index, term := range terms {
		if err := validateWebLine("search term", term, input.MaxTermBytes); err != nil {
			return fmt.Errorf("accepted search term %d: %w", index, err)
		}
		identity := strings.ToLower(term)
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf(
				"accepted web search term %q repeats an attempted or accepted query", term,
			)
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func BuildWebSearchTermCoveragePrompt(input WebSearchTermLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := marshalObjectiveContextInputForModel(input, input.Context)
	if err != nil {
		return "", fmt.Errorf("encode web search term coverage authority: %w", err)
	}
	return strings.Join([]string{
		"Answer one semantic coverage relation: is one more distinct alternate web query needed to search for evidence answering the exact question, after the attempted queries and accepted terms?",
		"Return exactly QUERY_TERM_REMAINS or NO_UNCOVERED_QUERY_TERM.",
		"Return no JSON, quotes, label, explanation, Markdown, or commentary.",
		"WEB_SEARCH_TERM_COVERAGE_AUTHORITY:\n" + string(projection),
	}, "\n\n"), nil
}

func BuildWebSearchTermPrompt(input WebSearchTermLeafInput) (string, error) {
	if err := input.validateForTerm(); err != nil {
		return "", err
	}
	projection, err := marshalObjectiveContextInputForModel(input, input.Context)
	if err != nil {
		return "", fmt.Errorf("encode web search term authority: %w", err)
	}
	return strings.Join([]string{
		"Return exactly one concise alternate web query needed to search for evidence answering the exact question that is not semantically covered by the attempted queries or accepted terms.",
		"Return only the query as one raw line. Do not return JSON, quotes, a label, Markdown, explanation, or commentary.",
		"WEB_SEARCH_TERM_AUTHORITY:\n" + string(projection),
	}, "\n\n"), nil
}

func DecodeWebSearchTermCoverageLeaf(
	input WebSearchTermLeafInput,
	raw string,
) (WebSearchTermCoverageDecision, error) {
	if err := input.validate(); err != nil {
		return WebSearchTermCoverageDecision{}, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"web search term coverage", raw, len(WebNoUncoveredQueryTerm), false,
	)
	if err != nil {
		return WebSearchTermCoverageDecision{}, err
	}
	decision := WebSearchTermCoverageDecision{Coverage: WebSearchTermCoverage(leaf)}
	switch decision.Coverage {
	case WebQueryTermRemains, WebNoUncoveredQueryTerm:
		return decision, nil
	default:
		return WebSearchTermCoverageDecision{}, fmt.Errorf(
			"web search term coverage %q is unsupported", decision.Coverage,
		)
	}
}

func DecodeWebSearchTermLeaf(
	input WebSearchTermLeafInput,
	raw string,
) (WebSearchTermDecision, error) {
	if err := input.validateForTerm(); err != nil {
		return WebSearchTermDecision{}, err
	}
	leaf, err := decodeRawSemanticLeaf(
		"web search term", raw, input.MaxTermBytes, false,
	)
	if err != nil {
		return WebSearchTermDecision{}, err
	}
	accepted := append(append([]string{}, input.AcceptedTerms...), leaf)
	if err := validateAcceptedWebSearchTerms(input.base(), accepted); err != nil {
		return WebSearchTermDecision{}, err
	}
	return WebSearchTermDecision{Term: leaf}, nil
}

func AssembleWebSearchTermsDecision(
	input WebSearchTermsInput,
	terms []string,
) (WebSearchTermsDecision, error) {
	decision := WebSearchTermsDecision{
		Schema: WebSearchTermsSchemaV1,
		Terms:  append([]string{}, terms...),
	}
	if err := decision.ValidateFor(input); err != nil {
		return WebSearchTermsDecision{}, err
	}
	return decision, nil
}
