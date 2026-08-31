package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MaxApplicationRequirementLeaves = maxRequirementCount
)

type ApplicationIntentInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
}

type ApplicationIntentCandidateRequirement struct {
	Statement string `json:"statement"`
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

func validateApplicationIntentText(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) || !utf8.ValidString(value) ||
		strings.ContainsRune(value, '\x00') || len(value) > maximum {
		return fmt.Errorf("application intent %s must be trimmed UTF-8 text of at most %d bytes", label, maximum)
	}
	return ValidatePathFreeModelContext("application intent "+label, value)
}
