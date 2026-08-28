package assemblyline

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	ApplicationProjectStackConstraintSchemaV1 = "omnidex.application-project-stack-constraint.v1"
	ApplicationProjectStackUnconstrained      = "UNCONSTRAINED"
	ApplicationProjectStackUnsupported        = "UNSUPPORTED"
	maxApplicationProjectStackCandidates      = 8
	maxApplicationProjectStackSummaryBytes    = 1024
)

var opaqueApplicationProjectStackPattern = regexp.MustCompile(`^STACK_CANDIDATE_[1-9][0-9]*$`)

// ApplicationProjectStackCandidate is a bounded semantic projection of a
// code-owned stack. Its physical identity and operational hooks stay outside
// the model-visible envelope.
type ApplicationProjectStackCandidate struct {
	CandidateID     string `json:"candidate_id"`
	TechnicalFormat string `json:"technical_format"`
}

type ApplicationProjectStackConstraintInput struct {
	ProductContext       string                             `json:"product_context"`
	AcceptedRequirements []string                           `json:"accepted_requirements"`
	Candidates           []ApplicationProjectStackCandidate `json:"candidates"`
}

type ApplicationProjectStackConstraintDecision struct {
	Schema      string `json:"schema"`
	CandidateID string `json:"candidate_id"`
}

func NewApplicationProjectStackConstraintJob(
	input ApplicationProjectStackConstraintInput,
) (PortableJob, error) {
	return newValidatedPortableJob(WorkApplicationProjectStackConstraint, input, input.validate)
}

func (input ApplicationProjectStackConstraintInput) validate() error {
	if err := validateApplicationProductQuote("project stack constraint", input.ProductContext); err != nil {
		return err
	}
	if len(input.AcceptedRequirements) < 1 || len(input.AcceptedRequirements) > maxRequirementCount {
		return fmt.Errorf("project stack constraint requires 1..%d accepted requirements", maxRequirementCount)
	}
	for index, requirement := range input.AcceptedRequirements {
		if err := validateApplicationIntentText(
			"project stack requirement", requirement, maxRequirementQuoteBytes,
		); err != nil {
			return fmt.Errorf("project stack constraint requirement %d: %w", index, err)
		}
	}
	if len(input.Candidates) < 1 || len(input.Candidates) > maxApplicationProjectStackCandidates {
		return fmt.Errorf(
			"project stack constraint requires 1..%d bounded candidates",
			maxApplicationProjectStackCandidates,
		)
	}
	seenFormats := make(map[string]struct{}, len(input.Candidates))
	for index, candidate := range input.Candidates {
		expectedID := fmt.Sprintf("STACK_CANDIDATE_%d", index+1)
		if candidate.CandidateID != expectedID ||
			!opaqueApplicationProjectStackPattern.MatchString(candidate.CandidateID) {
			return fmt.Errorf("project stack candidate %d has non-code-owned identity %q", index, candidate.CandidateID)
		}
		format := candidate.TechnicalFormat
		if format == "" || format != strings.TrimSpace(format) || !utf8.ValidString(format) ||
			len(format) > maxApplicationProjectStackSummaryBytes || strings.ContainsAny(format, "\x00\r\n") {
			return fmt.Errorf("project stack candidate %s has invalid technical format", candidate.CandidateID)
		}
		if err := ValidatePathFreeModelContext("project stack candidate technical format", format); err != nil {
			return err
		}
		if _, duplicate := seenFormats[format]; duplicate {
			return fmt.Errorf("project stack candidate %s repeats a technical format", candidate.CandidateID)
		}
		seenFormats[format] = struct{}{}
	}
	return nil
}

func (decision ApplicationProjectStackConstraintDecision) ValidateFor(
	input ApplicationProjectStackConstraintInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if decision.Schema != ApplicationProjectStackConstraintSchemaV1 {
		return fmt.Errorf(
			"project stack constraint schema must be %q",
			ApplicationProjectStackConstraintSchemaV1,
		)
	}
	if decision.CandidateID == ApplicationProjectStackUnconstrained ||
		decision.CandidateID == ApplicationProjectStackUnsupported {
		return nil
	}
	for _, candidate := range input.Candidates {
		if decision.CandidateID == candidate.CandidateID {
			return nil
		}
	}
	return fmt.Errorf("project stack candidate %q is unavailable", decision.CandidateID)
}

func BuildApplicationProjectStackConstraintPrompt(
	input ApplicationProjectStackConstraintInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := json.Marshal(struct {
		ProductContext       string   `json:"product_context"`
		AcceptedRequirements []string `json:"accepted_requirements"`
	}{input.ProductContext, input.AcceptedRequirements})
	if err != nil {
		return "", fmt.Errorf("encode project stack constraint authority: %w", err)
	}
	candidates, err := json.Marshal(input.Candidates)
	if err != nil {
		return "", fmt.Errorf("encode project stack candidates: %w", err)
	}
	return strings.Join([]string{
		"Determine which one registered technical format, if any, is explicitly required by the accepted application authority.",
		"Return one opaque candidate ID when exactly that format is required. Return UNCONSTRAINED when no technical format is explicit. Return UNSUPPORTED when an explicit or contradictory technical constraint cannot be satisfied by exactly one candidate.",
		"Return exactly that raw ID token and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"ACCEPTED_APPLICATION_AUTHORITY:\n" + string(authority),
		"REGISTERED_TECHNICAL_FORMATS:\n" + string(candidates),
	}, "\n\n"), nil
}

func DecodeApplicationProjectStackConstraintDecision(
	input ApplicationProjectStackConstraintInput,
	raw string,
) (ApplicationProjectStackConstraintDecision, error) {
	leaf, err := decodeRawSemanticLeaf("project stack candidate", raw, 64, false)
	if err != nil {
		return ApplicationProjectStackConstraintDecision{}, err
	}
	decision := ApplicationProjectStackConstraintDecision{
		Schema: ApplicationProjectStackConstraintSchemaV1, CandidateID: leaf,
	}
	if err := decision.ValidateFor(input); err != nil {
		return ApplicationProjectStackConstraintDecision{}, err
	}
	return decision, nil
}
