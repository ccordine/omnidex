package assemblyline

import (
	"fmt"
	"strings"
)

const (
	WorkWebReviewClaimCoverage WorkKind = "web_review_claim_coverage"
	WorkWebReviewClaim         WorkKind = "web_review_claim"

	WebReviewClaimRemains     WebReviewClaimCoverage = "CLAIM_REMAINS"
	WebReviewNoUncoveredClaim WebReviewClaimCoverage = "NO_UNCOVERED_CLAIM"

	MaxWebReviewClaims     = 12
	maxWebReviewClaimBytes = 1024
)

type WebReviewClaimCoverage string

type WebReviewClaimLeafInput struct {
	ExactQuestion  string           `json:"exact_question"`
	Context        ObjectiveContext `json:"objective_context"`
	ParagraphText  string           `json:"paragraph_text"`
	AcceptedClaims []string         `json:"accepted_claims"`
}

type WebReviewClaimCoverageDecision struct {
	Coverage WebReviewClaimCoverage `json:"coverage"`
}

type WebReviewClaimDecision struct {
	Claim string `json:"claim"`
}

func NewWebReviewClaimCoverageJob(
	input WebReviewClaimLeafInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkWebReviewClaimCoverage, input, input.validate)
}

func NewWebReviewClaimJob(input WebReviewClaimLeafInput) (PortableJob, error) {
	return newValidatedPortableJob(WorkWebReviewClaim, input, input.validate)
}

func (input WebReviewClaimLeafInput) validate() error {
	if err := validateExactWebQuestion(input.ExactQuestion); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if input.ParagraphText != strings.TrimSpace(input.ParagraphText) {
		return fmt.Errorf("web review claim paragraph must be exactly trimmed")
	}
	if err := validateWebText(
		"paragraph text", input.ParagraphText, maxWebSynthesisParagraphBytes, true,
	); err != nil {
		return err
	}
	if webModelCitationSyntax.MatchString(input.ParagraphText) {
		return fmt.Errorf("web review claim paragraph contains model-authored citation syntax")
	}
	if input.AcceptedClaims == nil {
		return fmt.Errorf("web review claim leaf requires a non-nil accepted set")
	}
	if len(input.AcceptedClaims) > MaxWebReviewClaims {
		return fmt.Errorf("web review claim leaf exceeds %d accepted claims", MaxWebReviewClaims)
	}
	seen := make(map[string]struct{}, len(input.AcceptedClaims))
	for index, claim := range input.AcceptedClaims {
		if err := validateWebReviewClaim(claim); err != nil {
			return fmt.Errorf("accepted web review claim %d: %w", index, err)
		}
		if _, duplicate := seen[claim]; duplicate {
			return fmt.Errorf("accepted web review claim %d is duplicated", index)
		}
		seen[claim] = struct{}{}
	}
	return nil
}

func validateWebReviewClaim(claim string) error {
	if claim != strings.TrimSpace(claim) || strings.ContainsAny(claim, "\r\n") {
		return fmt.Errorf("web review claim must be one exactly trimmed line")
	}
	return validateWebText("review claim", claim, maxWebReviewClaimBytes, true)
}

func (decision WebReviewClaimCoverageDecision) ValidateFor(
	input WebReviewClaimLeafInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	switch decision.Coverage {
	case WebReviewClaimRemains, WebReviewNoUncoveredClaim:
		return nil
	default:
		return fmt.Errorf("web review claim coverage %q is unsupported", decision.Coverage)
	}
}

func (decision WebReviewClaimDecision) ValidateFor(
	input WebReviewClaimLeafInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if err := validateWebReviewClaim(decision.Claim); err != nil {
		return err
	}
	for _, accepted := range input.AcceptedClaims {
		if decision.Claim == accepted {
			return fmt.Errorf("web review claim duplicates an accepted claim")
		}
	}
	return nil
}

func BuildWebReviewClaimCoveragePrompt(
	input WebReviewClaimLeafInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	projection, err := marshalObjectiveContextInputForModel(input, input.Context)
	if err != nil {
		return "", fmt.Errorf("encode web review claim coverage authority: %w", err)
	}
	return strings.Join([]string{
		"Answer one semantic question: does the exact paragraph contain one more distinct material factual claim not already represented by the accepted claims?",
		"Return exactly CLAIM_REMAINS or NO_UNCOVERED_CLAIM.",
		"Return no JSON, quotes, label, explanation, or commentary.",
		"CLAIM_COVERAGE_AUTHORITY:\n" + string(projection),
	}, "\n\n"), nil
}

func BuildWebReviewClaimPrompt(input WebReviewClaimLeafInput) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	if len(input.AcceptedClaims) >= MaxWebReviewClaims {
		return "", fmt.Errorf("web review claim bound is exhausted")
	}
	projection, err := marshalObjectiveContextInputForModel(input, input.Context)
	if err != nil {
		return "", fmt.Errorf("encode web review claim authority: %w", err)
	}
	return strings.Join([]string{
		"Identify exactly one next distinct material factual claim made by the exact paragraph and not represented by the accepted claims.",
		"Return only that claim as one raw line. Do not return a verdict, evidence ID, JSON, quotes, label, Markdown, or commentary.",
		"CLAIM_AUTHORITY:\n" + string(projection),
	}, "\n\n"), nil
}

func DecodeWebReviewClaimCoverageDecision(
	input WebReviewClaimLeafInput,
	raw string,
) (WebReviewClaimCoverageDecision, error) {
	leaf, err := decodeRawSemanticLeaf(
		"web review claim coverage", raw, len(WebReviewNoUncoveredClaim), false,
	)
	if err != nil {
		return WebReviewClaimCoverageDecision{}, err
	}
	decision := WebReviewClaimCoverageDecision{
		Coverage: WebReviewClaimCoverage(leaf),
	}
	if err := decision.ValidateFor(input); err != nil {
		return WebReviewClaimCoverageDecision{}, err
	}
	return decision, nil
}

func DecodeWebReviewClaimDecision(
	input WebReviewClaimLeafInput,
	raw string,
) (WebReviewClaimDecision, error) {
	leaf, err := decodeRawSemanticLeaf(
		"web review claim", raw, maxWebReviewClaimBytes, false,
	)
	if err != nil {
		return WebReviewClaimDecision{}, err
	}
	decision := WebReviewClaimDecision{Claim: leaf}
	if err := decision.ValidateFor(input); err != nil {
		return WebReviewClaimDecision{}, err
	}
	return decision, nil
}
