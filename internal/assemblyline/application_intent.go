package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const ApplicationIntentCandidateSchemaV1 = "omnidex.application-intent.v1"

type ApplicationIntentInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
}

type ApplicationIntentCandidate struct {
	Schema         string   `json:"schema"`
	ProductContext string   `json:"product_context"`
	Requirements   []string `json:"requirements"`
}

type ApplicationRequirement struct {
	ID            string `json:"id"`
	Statement     string `json:"statement"`
	RequestSHA256 string `json:"request_sha256"`
}

type ApplicationIntentResolution struct {
	ProductContext string                   `json:"product_context"`
	RequestSHA256  string                   `json:"request_sha256"`
	Requirements   []ApplicationRequirement `json:"requirements"`
}

func (input ApplicationIntentInput) validate() error {
	if err := validateApplicationRequest("application intent", input.UserRequest); err != nil {
		return err
	}
	if err := ValidatePathFreeModelContext("application intent request", input.UserRequest); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if input.Context.RequestSHA256 != ExactObjectiveContextSHA(input.UserRequest) {
		return fmt.Errorf("application intent request does not match context authority")
	}
	return nil
}

func (candidate ApplicationIntentCandidate) Validate() error {
	if candidate.Schema != ApplicationIntentCandidateSchemaV1 {
		return fmt.Errorf("application intent schema must be %q", ApplicationIntentCandidateSchemaV1)
	}
	if err := validateApplicationIntentText(
		"product context", candidate.ProductContext, maxApplicationProductBytes,
	); err != nil {
		return err
	}
	if candidate.Requirements == nil {
		return fmt.Errorf("application intent requirements must be an array")
	}
	if len(candidate.Requirements) < 1 || len(candidate.Requirements) > maxRequirementCount {
		return fmt.Errorf(
			"application intent requires between 1 and %d requirement statements",
			maxRequirementCount,
		)
	}
	seen := make(map[string]struct{}, len(candidate.Requirements))
	for index, statement := range candidate.Requirements {
		if err := validateApplicationIntentText(
			"requirement statement", statement, maxRequirementQuoteBytes,
		); err != nil {
			return fmt.Errorf("application intent requirement %d: %w", index, err)
		}
		if _, duplicate := seen[statement]; duplicate {
			return fmt.Errorf("application intent requirement %d is duplicated", index)
		}
		seen[statement] = struct{}{}
	}
	return nil
}

func (candidate ApplicationIntentCandidate) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	values := append([]string{candidate.ProductContext}, candidate.Requirements...)
	return ValidatePathFreeModelContextWithProvenance(
		"application intent candidate", provenance, values...,
	)
}

func ResolveApplicationIntent(
	input ApplicationIntentInput,
	candidate ApplicationIntentCandidate,
) (ApplicationIntentResolution, error) {
	var zero ApplicationIntentResolution
	if err := input.validate(); err != nil {
		return zero, err
	}
	if err := candidate.Validate(); err != nil {
		return zero, err
	}
	requirements := make([]ApplicationRequirement, len(candidate.Requirements))
	for index, statement := range candidate.Requirements {
		requirements[index] = ApplicationRequirement{
			ID: fmt.Sprintf("requirement_%03d", index+1), Statement: statement,
			RequestSHA256: input.Context.RequestSHA256,
		}
	}
	return ApplicationIntentResolution{
		ProductContext: candidate.ProductContext,
		RequestSHA256:  input.Context.RequestSHA256,
		Requirements:   requirements,
	}, nil
}

func cloneApplicationIntentCandidate(candidate ApplicationIntentCandidate) ApplicationIntentCandidate {
	candidate.Requirements = append([]string(nil), candidate.Requirements...)
	return candidate
}

func validateApplicationIntentText(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > maximum {
		return fmt.Errorf("application intent %s must be trimmed UTF-8 text of at most %d bytes", label, maximum)
	}
	return ValidatePathFreeModelContext("application intent "+label, value)
}
