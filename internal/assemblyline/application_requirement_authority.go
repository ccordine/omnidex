package assemblyline

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	WorkApplicationRequirementCoverage          WorkKind = "application_requirement_coverage"
	WorkApplicationRequirement                  WorkKind = "application_requirement"
	MaxApplicationRequirementLeaves                      = maxRequirementCount
	ApplicationRequirementRemains                        = "REQUIREMENT_REMAINS"
	ApplicationNoUncoveredRequirement                    = "NO_UNCOVERED_REQUIREMENT"
	ApplicationRequirementCoverageSchemaV1               = "omnidex.application-requirement-coverage.v1"
	MaxApplicationRequirementExcludedCandidates          = MaxApplicationRequirementLeaves
)

type ApplicationRequirementCoverageInput struct {
	UserRequest          string             `json:"user_request"`
	Context              ApplicationContext `json:"context"`
	AcceptedRequirements []string           `json:"accepted_requirements"`
	ExcludedCandidates   []string           `json:"excluded_candidates"`
}

type ApplicationRequirementCoverageResult struct {
	Schema          string `json:"schema"`
	AuthoritySHA256 string `json:"authority_sha256"`
	Relation        string `json:"relation"`
}

type ApplicationRequirementCandidateInput struct {
	Authority ApplicationRequirementCoverageInput  `json:"authority"`
	Coverage  ApplicationRequirementCoverageResult `json:"coverage"`
}

func NewApplicationRequirementCoverageJob(
	input ApplicationRequirementCoverageInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirementCoverage, input, input.validate,
	)
}

func NewApplicationRequirementJob(
	input ApplicationRequirementCandidateInput,
) (PortableJob, error) {
	return newValidatedPortableJob(
		WorkApplicationRequirement, input, input.validate,
	)
}

func (input ApplicationRequirementCoverageInput) validate() error {
	if err := (ApplicationIntentInput{
		UserRequest: input.UserRequest,
		Context:     input.Context,
	}).validate(); err != nil {
		return err
	}
	if input.AcceptedRequirements == nil {
		return fmt.Errorf("application requirement coverage requires a non-nil accepted set")
	}
	if input.ExcludedCandidates == nil {
		return fmt.Errorf("application requirement coverage requires a non-nil excluded candidate set")
	}
	if len(input.AcceptedRequirements) > maxRequirementCount {
		return fmt.Errorf(
			"application requirement leaf exceeds %d accepted requirements",
			maxRequirementCount,
		)
	}
	seen := make(map[string]struct{}, len(input.AcceptedRequirements))
	for index, requirement := range input.AcceptedRequirements {
		if err := validateApplicationIntentText(
			"requirement statement", requirement, maxRequirementQuoteBytes,
		); err != nil {
			return fmt.Errorf("accepted application requirement %d: %w", index, err)
		}
		if _, duplicate := seen[requirement]; duplicate {
			return fmt.Errorf("accepted application requirement %d is duplicated", index)
		}
		seen[requirement] = struct{}{}
	}
	if len(input.ExcludedCandidates) > MaxApplicationRequirementExcludedCandidates {
		return fmt.Errorf(
			"application requirement leaf exceeds %d excluded candidates",
			MaxApplicationRequirementExcludedCandidates,
		)
	}
	for index, candidate := range input.ExcludedCandidates {
		if err := validateApplicationIntentText(
			"excluded application requirement candidate", candidate,
			maxRequirementQuoteBytes,
		); err != nil {
			return fmt.Errorf("excluded application requirement candidate %d: %w", index, err)
		}
		if _, duplicate := seen[candidate]; duplicate {
			return fmt.Errorf(
				"excluded application requirement candidate %d duplicates retained authority",
				index,
			)
		}
		seen[candidate] = struct{}{}
	}
	return nil
}

func (result ApplicationRequirementCoverageResult) ValidateFor(
	input ApplicationRequirementCoverageInput,
) error {
	if err := input.validate(); err != nil {
		return err
	}
	if result.Schema != ApplicationRequirementCoverageSchemaV1 {
		return fmt.Errorf(
			"application requirement coverage schema must be %q",
			ApplicationRequirementCoverageSchemaV1,
		)
	}
	authoritySHA256, err := applicationRequirementCoverageAuthoritySHA256(input)
	if err != nil {
		return err
	}
	if result.AuthoritySHA256 != authoritySHA256 {
		return fmt.Errorf("application requirement coverage authority hash does not match")
	}
	switch result.Relation {
	case ApplicationRequirementRemains, ApplicationNoUncoveredRequirement:
		return nil
	default:
		return fmt.Errorf(
			"application requirement coverage value %q is not registered",
			result.Relation,
		)
	}
}

func (input ApplicationRequirementCandidateInput) validate() error {
	if err := input.Coverage.ValidateFor(input.Authority); err != nil {
		return fmt.Errorf("validate application requirement candidate coverage: %w", err)
	}
	if input.Coverage.Relation != ApplicationRequirementRemains {
		return fmt.Errorf(
			"application requirement generation requires code-established relation %q",
			ApplicationRequirementRemains,
		)
	}
	return nil
}

// RebindApplicationRequirementAfterNonRuntimeExclusion derives the next
// generation authority without another coverage call. The prior receipt proves
// that a runtime requirement remains, while the candidate-bound kind receipt
// proves that adding this exact candidate cannot cover that runtime requirement.
func RebindApplicationRequirementAfterNonRuntimeExclusion(
	input ApplicationRequirementCandidateInput,
	candidate string,
	kind ApplicationRequirementCandidateKindResult,
) (ApplicationRequirementCandidateInput, error) {
	var zero ApplicationRequirementCandidateInput
	if err := input.validate(); err != nil {
		return zero, err
	}
	kindInput := ApplicationRequirementCandidateKindInput{Candidate: candidate}
	if err := kind.ValidateFor(kindInput); err != nil {
		return zero, fmt.Errorf(
			"validate application requirement exclusion kind: %w", err,
		)
	}
	if kind.Relation != ApplicationRequirementCandidateNonRuntime {
		return zero, fmt.Errorf(
			"application requirement exclusion requires code-established relation %q",
			ApplicationRequirementCandidateNonRuntime,
		)
	}
	if len(input.Authority.ExcludedCandidates) == MaxApplicationRequirementExcludedCandidates {
		return zero, fmt.Errorf(
			"application requirement exclusions reached the code-owned %d-item bound",
			MaxApplicationRequirementExcludedCandidates,
		)
	}
	for _, retained := range input.Authority.AcceptedRequirements {
		if candidate == retained {
			return zero, fmt.Errorf(
				"non-runtime exclusion duplicates an accepted requirement",
			)
		}
	}
	for _, excluded := range input.Authority.ExcludedCandidates {
		if candidate == excluded {
			return zero, fmt.Errorf(
				"non-runtime exclusion duplicates an excluded candidate",
			)
		}
	}
	authority := input.Authority
	authority.AcceptedRequirements = append(
		[]string{}, input.Authority.AcceptedRequirements...,
	)
	authority.ExcludedCandidates = append(
		append([]string{}, input.Authority.ExcludedCandidates...), candidate,
	)
	authoritySHA256, err := applicationRequirementCoverageAuthoritySHA256(authority)
	if err != nil {
		return zero, err
	}
	rebound := ApplicationRequirementCandidateInput{
		Authority: authority,
		Coverage: ApplicationRequirementCoverageResult{
			Schema:          ApplicationRequirementCoverageSchemaV1,
			AuthoritySHA256: authoritySHA256,
			Relation:        ApplicationRequirementRemains,
		},
	}
	if err := rebound.validate(); err != nil {
		return zero, fmt.Errorf(
			"validate rebound application requirement authority: %w", err,
		)
	}
	return rebound, nil
}

func applicationRequirementCoverageAuthoritySHA256(
	input ApplicationRequirementCoverageInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	authority, err := exactjson.Canonical(input)
	if err != nil {
		return "", fmt.Errorf("encode application requirement coverage authority: %w", err)
	}
	return ExactObjectiveContextSHA(string(authority)), nil
}

func applicationRequirementCoverageProjection(
	input ApplicationRequirementCoverageInput,
) (string, error) {
	if err := input.validate(); err != nil {
		return "", err
	}
	var projection strings.Builder
	projection.WriteString(renderApplicationContextModelProjection(
		input.UserRequest,
		input.Context,
	))
	projection.WriteByte('\n')
	if len(input.AcceptedRequirements) == 0 {
		projection.WriteString("ACCEPTED REQUIREMENTS:\n(none)\n")
	} else {
		for index, requirement := range input.AcceptedRequirements {
			fmt.Fprintf(
				&projection,
				"ACCEPTED REQUIREMENT %d:\n%s\n",
				index+1,
				requirement,
			)
		}
	}
	if projection.Len() > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application requirement projection exceeds %d bytes",
			maxPortablePayloadBytes,
		)
	}
	return strings.TrimSuffix(projection.String(), "\n"), nil
}

func applicationRequirementGenerationProjection(
	input ApplicationRequirementCoverageInput,
) (string, error) {
	projection, err := applicationRequirementCoverageProjection(input)
	if err != nil {
		return "", err
	}
	var generation strings.Builder
	generation.WriteString(projection)
	generation.WriteByte('\n')
	if len(input.ExcludedCandidates) == 0 {
		generation.WriteString("EXCLUDED NON-RUNTIME CANDIDATES:\n(none)\n")
	} else {
		for index, candidate := range input.ExcludedCandidates {
			fmt.Fprintf(
				&generation,
				"EXCLUDED NON-RUNTIME CANDIDATE %d:\n%s\n",
				index+1,
				candidate,
			)
		}
	}
	if generation.Len() > maxPortablePayloadBytes {
		return "", fmt.Errorf(
			"application requirement generation projection exceeds %d bytes",
			maxPortablePayloadBytes,
		)
	}
	return strings.TrimSuffix(generation.String(), "\n"), nil
}
