package assemblyline

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	ApplicationProjectStackConstraintSchemaV2 = "omnidex.application-project-stack-constraint.v2"
	ApplicationProjectStackUnconstrained      = "UNCONSTRAINED"
	ApplicationProjectStackUnsupported        = "UNSUPPORTED"
	maxApplicationProjectStackCandidates      = 8
	maxApplicationProjectStackSummaryBytesV2  = 2048
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
	UserRequest string                             `json:"user_request"`
	Candidates  []ApplicationProjectStackCandidate `json:"candidates"`
}

type ApplicationProjectStackConstraintDecision struct {
	Schema      string `json:"schema"`
	CandidateID string `json:"candidate_id"`
}

func NewApplicationProjectStackConstraintJob(
	input ApplicationProjectStackConstraintInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationProjectStackConstraint, input, input.validate,
	)
}

func (input ApplicationProjectStackConstraintInput) validate() error {
	if err := validateApplicationRequest("project stack constraint", input.UserRequest); err != nil {
		return err
	}
	if err := ValidatePathFreeModelContext(
		"project stack constraint request", input.UserRequest,
	); err != nil {
		return err
	}
	return validateApplicationProjectStackCandidates(
		input.Candidates, maxApplicationProjectStackSummaryBytesV2,
	)
}

func validateApplicationProjectStackCandidates(
	candidates []ApplicationProjectStackCandidate,
	maximumSummaryBytes int,
) error {
	if len(candidates) < 1 || len(candidates) > maxApplicationProjectStackCandidates {
		return fmt.Errorf(
			"project stack constraint requires 1..%d bounded candidates",
			maxApplicationProjectStackCandidates,
		)
	}
	seenFormats := make(map[string]struct{}, len(candidates))
	for index, candidate := range candidates {
		expectedID := fmt.Sprintf("STACK_CANDIDATE_%d", index+1)
		if candidate.CandidateID != expectedID ||
			!opaqueApplicationProjectStackPattern.MatchString(candidate.CandidateID) {
			return fmt.Errorf("project stack candidate %d has non-code-owned identity %q", index, candidate.CandidateID)
		}
		format := candidate.TechnicalFormat
		if format == "" || format != strings.TrimSpace(format) || !utf8.ValidString(format) ||
			strings.ContainsAny(format, "\x00\r\n") {
			return fmt.Errorf("project stack candidate %s has invalid technical format", candidate.CandidateID)
		}
		if len(format) > maximumSummaryBytes {
			return fmt.Errorf(
				"project stack candidate %s technical format exceeds %d bytes",
				candidate.CandidateID, maximumSummaryBytes,
			)
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
	if decision.Schema != ApplicationProjectStackConstraintSchemaV2 {
		return fmt.Errorf(
			"project stack constraint schema must be %q",
			ApplicationProjectStackConstraintSchemaV2,
		)
	}
	return validateApplicationProjectStackDecisionCandidates(
		decision.CandidateID, input.Candidates,
	)
}

func validateApplicationProjectStackDecisionCandidates(
	candidateID string,
	candidates []ApplicationProjectStackCandidate,
) error {
	if candidateID == ApplicationProjectStackUnconstrained ||
		candidateID == ApplicationProjectStackUnsupported {
		return nil
	}
	for _, candidate := range candidates {
		if candidateID == candidate.CandidateID {
			return nil
		}
	}
	return fmt.Errorf("project stack candidate %q is unavailable", candidateID)
}

func BuildApplicationProjectStackConstraintPrompt(
	input ApplicationProjectStackConstraintInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := json.Marshal(struct {
		UserRequest string `json:"user_request"`
	}{input.UserRequest})
	if err != nil {
		return "", fmt.Errorf("encode project stack constraint authority: %w", err)
	}
	candidates, err := json.Marshal(input.Candidates)
	if err != nil {
		return "", fmt.Errorf("encode project stack candidates: %w", err)
	}
	return strings.Join([]string{
		"Determine which one registered technical format and packaging shape, if any, is explicitly required by the immutable software request.",
		"Return one opaque candidate ID when exactly that format is required. Return UNCONSTRAINED when no technical format is explicit. Return UNSUPPORTED when an explicit or contradictory technical constraint cannot be satisfied by exactly one candidate.",
		"Return exactly that raw ID token and nothing else: no JSON, quotes, label, Markdown, or commentary.",
		"IMMUTABLE_SOFTWARE_REQUEST:\n" + string(authority),
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
		Schema: ApplicationProjectStackConstraintSchemaV2, CandidateID: leaf,
	}
	if err := decision.ValidateFor(input); err != nil {
		return ApplicationProjectStackConstraintDecision{}, err
	}
	return decision, nil
}
