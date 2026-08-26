package assemblyline

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const RepositoryRequirementInterpretationSchemaV2 = "omnidex.repository-requirements.v2"

type RepositoryRequirementInterpretationInput struct {
	UserRequest string             `json:"user_request"`
	Context     ApplicationContext `json:"context"`
}

type RepositoryRequirementInterpretation struct {
	Schema       string   `json:"schema"`
	Requirements []string `json:"requirements"`
}

func (input RepositoryRequirementInterpretationInput) validate() error {
	if err := validateApplicationRequest("repository requirements", input.UserRequest); err != nil {
		return err
	}
	if err := ValidatePathFreeModelContext("repository requirement request", input.UserRequest); err != nil {
		return err
	}
	if err := input.Context.Validate(); err != nil {
		return err
	}
	if input.Context.WorkspaceState != ApplicationWorkspaceExisting {
		return fmt.Errorf("repository requirements require an existing-workspace context")
	}
	if input.Context.RequestSHA256 != ExactObjectiveContextSHA(input.UserRequest) {
		return fmt.Errorf("repository requirements request does not match context authority")
	}
	return nil
}

func ResolveRepositoryRequirements(
	input RepositoryRequirementInterpretationInput,
	interpretation RepositoryRequirementInterpretation,
) ([]string, error) {
	if err := input.validate(); err != nil {
		return nil, err
	}
	if err := interpretation.ValidateFor(input); err != nil {
		return nil, err
	}
	return append([]string(nil), interpretation.Requirements...), nil
}

func (interpretation RepositoryRequirementInterpretation) ValidateFor(
	input RepositoryRequirementInterpretationInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if interpretation.Schema != RepositoryRequirementInterpretationSchemaV2 {
		return fmt.Errorf(
			"repository requirement schema must be %q",
			RepositoryRequirementInterpretationSchemaV2,
		)
	}
	if interpretation.Requirements == nil {
		return fmt.Errorf("repository requirements must be an array")
	}
	if len(interpretation.Requirements) < 1 || len(interpretation.Requirements) > maxRequirementCount {
		return fmt.Errorf(
			"repository requirements must contain between 1 and %d statements",
			maxRequirementCount,
		)
	}

	seen := make(map[string]struct{}, len(interpretation.Requirements))
	for index, requirement := range interpretation.Requirements {
		label := fmt.Sprintf("repository requirement %d", index)
		if err := validateRepositoryRequirementStatement(label, requirement); err != nil {
			return err
		}
		if _, duplicate := seen[requirement]; duplicate {
			return fmt.Errorf("%s duplicates %q", label, requirement)
		}
		seen[requirement] = struct{}{}
	}
	return nil
}

func (interpretation RepositoryRequirementInterpretation) ValidatePathFree(
	provenance ArtifactIdentityProvenance,
) error {
	return ValidatePathFreeModelContextWithProvenance(
		"repository requirement interpretation", provenance,
		interpretation.Requirements...,
	)
}

func validateRepositoryRequirementStatement(label, statement string) error {
	if statement == "" || statement != strings.TrimSpace(statement) {
		return fmt.Errorf("%s must be one non-empty trimmed statement", label)
	}
	if !utf8.ValidString(statement) || strings.ContainsRune(statement, '\x00') {
		return fmt.Errorf("%s must be valid UTF-8 without NUL bytes", label)
	}
	if len([]byte(statement)) > maxRequirementQuoteBytes {
		return fmt.Errorf("%s exceeds %d bytes", label, maxRequirementQuoteBytes)
	}
	return ValidatePathFreeModelContext(label, statement)
}
