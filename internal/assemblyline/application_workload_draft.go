package assemblyline

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type applicationWorkloadRequirementAuthority struct {
	ID          string `json:"requirement_id"`
	SourceQuote string `json:"source_quote"`
}

func validateApplicationWorkloadDraftInput(input ApplicationWorkloadDraftInput) error {
	return validateApplicationAuthority(
		"application workload", input.Surface, input.ProductQuote, input.Requirements,
	)
}

func validateApplicationAuthority(
	label string,
	surface ApplicationSurface,
	productQuote string,
	requirements []Requirement,
) error {
	switch surface {
	case ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine, ApplicationSurfaceService:
	case ApplicationSurfaceUnsupported:
		return fmt.Errorf("%s cannot use unsupported surface", label)
	default:
		return fmt.Errorf("%s surface %q is unsupported", label, surface)
	}
	if err := validateApplicationProductQuote(label, productQuote); err != nil {
		return err
	}
	if !utf8.ValidString(productQuote) || strings.ContainsRune(productQuote, '\x00') {
		return fmt.Errorf("%s product quote must be valid NUL-free UTF-8", label)
	}
	if len(requirements) < 1 || len(requirements) > maxRequirementCount {
		return fmt.Errorf("%s requires 1..%d accepted requirements", label, maxRequirementCount)
	}
	seen := make(map[string]struct{}, len(requirements))
	for index, requirement := range requirements {
		wantID := fmt.Sprintf("requirement_%03d", index+1)
		if requirement.ID != wantID {
			return fmt.Errorf("%s requirement %d identity must be %q", label, index, wantID)
		}
		if err := validateRequirementQuote(label+" requirement", requirement.SourceQuote); err != nil {
			return fmt.Errorf("%s: %w", requirement.ID, err)
		}
		if !utf8.ValidString(requirement.SourceQuote) || strings.ContainsRune(requirement.SourceQuote, '\x00') {
			return fmt.Errorf("%s source quote must be valid NUL-free UTF-8", requirement.ID)
		}
		if _, duplicate := seen[requirement.SourceQuote]; duplicate {
			return fmt.Errorf("%s requirement quote %q is duplicated", label, requirement.SourceQuote)
		}
		seen[requirement.SourceQuote] = struct{}{}
	}
	return nil
}

func MaterializeApplicationWorkloadDraft(
	authority ApplicationWorkloadDraftInput,
	specifications []ApplicationJobSpecification,
) (ApplicationWorkloadDraft, error) {
	var zero ApplicationWorkloadDraft
	if err := validateApplicationWorkloadDraftInput(authority); err != nil {
		return zero, err
	}
	if len(specifications) != len(authority.Requirements) {
		return zero, fmt.Errorf(
			"application workload requires exactly %d job specifications", len(authority.Requirements),
		)
	}
	draft := ApplicationWorkloadDraft{
		Schema: ApplicationWorkloadDraftSchemaV1,
		Tasks:  make([]ApplicationWorkloadTaskDraft, 0, len(specifications)),
	}
	for index, specification := range specifications {
		if err := ValidateApplicationJobSpecification(specification); err != nil {
			return zero, fmt.Errorf("application workload job specification %d: %w", index, err)
		}
		draft.Tasks = append(draft.Tasks, ApplicationWorkloadTaskDraft{
			RequirementID:      authority.Requirements[index].ID,
			Objective:          specification.Objective,
			RequiredBehaviors:  append([]string(nil), specification.RequiredBehaviors...),
			AcceptanceCriteria: append([]string(nil), specification.AcceptanceCriteria...),
			DependsOn:          []string{},
		})
	}
	return draft, nil
}

func validateApplicationJobSpecificationList(
	label string,
	values []string,
	maximumCount int,
	maximumRunes int,
) error {
	if len(values) < 1 || len(values) > maximumCount {
		return fmt.Errorf("requires 1..%d %ss", maximumCount, label)
	}
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if err := validateApplicationWorkloadLine(label, value, maximumRunes); err != nil {
			return fmt.Errorf("%s %d: %w", label, index, err)
		}
		if _, duplicate := seen[value]; duplicate {
			return fmt.Errorf("%s %q is duplicated", label, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateApplicationWorkloadLine(label, value string, maximum int) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must be one non-empty trimmed line", label)
	}
	if utf8.RuneCountInString(value) > maximum {
		return fmt.Errorf("%s exceeds %d runes", label, maximum)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("%s must be valid NUL-free UTF-8", label)
	}
	for _, valueRune := range value {
		if unicode.IsControl(valueRune) {
			return fmt.Errorf("%s must not contain control characters", label)
		}
	}
	if err := ValidatePathFreeModelContext(label, value); err != nil {
		return err
	}
	return nil
}

func validateApplicationWorkloadIdentifier(label, value string) error {
	return validateApplicationWorkloadLine(label, value, maxApplicationDependencyIDBytes)
}
