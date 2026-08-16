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
	if interpretation.Schema != RepositoryRequirementInterpretationSchemaV2 {
		return nil, fmt.Errorf(
			"repository requirement schema must be %q",
			RepositoryRequirementInterpretationSchemaV2,
		)
	}
	if interpretation.Requirements == nil {
		return nil, fmt.Errorf("repository requirements must be an array")
	}
	if len(interpretation.Requirements) < 1 || len(interpretation.Requirements) > maxRequirementCount {
		return nil, fmt.Errorf(
			"repository requirements must contain between 1 and %d statements",
			maxRequirementCount,
		)
	}

	requirements := make([]string, len(interpretation.Requirements))
	seen := make(map[string]struct{}, len(interpretation.Requirements))
	for index, requirement := range interpretation.Requirements {
		label := fmt.Sprintf("repository requirement %d", index)
		if err := validateRepositoryRequirementStatement(label, requirement); err != nil {
			return nil, err
		}
		if _, duplicate := seen[requirement]; duplicate {
			return nil, fmt.Errorf("%s duplicates %q", label, requirement)
		}
		seen[requirement] = struct{}{}
		requirements[index] = requirement
	}
	return requirements, nil
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
	return nil
}
