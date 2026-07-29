package assemblyline

import (
	"fmt"
	"strings"
)

const ApplicationClassificationSchemaV1 = "omnidex.application-class.v1"
const ApplicationIdentitySchemaV1 = "omnidex.application-identity.v1"

const (
	RequirementPartitionSchemaV1 = "omnidex.requirement-partition.v1"
	ArtifactHandlingSchemaV1     = "omnidex.artifact-handling.v1"
	maxApplicationProductBytes   = 512
)

type ApplicationSurface string

const (
	ApplicationSurfaceBrowser     ApplicationSurface = "browser_application"
	ApplicationSurfaceCommandLine ApplicationSurface = "command_line_application"
	ApplicationSurfaceService     ApplicationSurface = "service_application"
	ApplicationSurfaceUnsupported ApplicationSurface = "unsupported"
)

type ApplicationClassification struct {
	Schema  string             `json:"schema"`
	Surface ApplicationSurface `json:"surface"`
}

type ApplicationIdentity struct {
	Schema       string `json:"schema"`
	ProductQuote string `json:"product_quote"`
}

type RequirementPartitionDecision struct {
	Schema        string   `json:"schema"`
	FeatureQuotes []string `json:"feature_quotes"`
}

type ArtifactHandlingDecision struct {
	Schema   string           `json:"schema"`
	Token    string           `json:"token"`
	Handling ArtifactHandling `json:"handling"`
}

type ApplicationSpecification struct {
	Surface      ApplicationSurface
	ProductQuote string
	Requirements []Requirement
	Artifacts    []ArtifactDirective
}

func (decision RequirementPartitionDecision) ValidateFor(input RequirementPartitionInput) error {
	if decision.Schema != RequirementPartitionSchemaV1 {
		return fmt.Errorf("requirement partition schema must be %q", RequirementPartitionSchemaV1)
	}
	if input.Mode == RequirementSplitFeature && len(decision.FeatureQuotes) == 0 {
		return fmt.Errorf("requirement feature split requires at least one feature quote")
	}
	if len(decision.FeatureQuotes) > maxRequirementPartitionCount {
		return fmt.Errorf("requirement partition exceeds %d feature quotes", maxRequirementPartitionCount)
	}
	accepted := make([]textSpan, 0, len(decision.FeatureQuotes))
	seen := make(map[string]struct{}, len(decision.FeatureQuotes))
	for index, quote := range decision.FeatureQuotes {
		if err := validateRequirementQuote("requirement partition feature", quote); err != nil {
			return fmt.Errorf("feature quote %d: %w", index, err)
		}
		if _, duplicate := seen[quote]; duplicate {
			return fmt.Errorf("feature quote %d duplicates %q", index, quote)
		}
		seen[quote] = struct{}{}
		span, err := uniqueTextSpan(input.SourceText, quote)
		if err != nil {
			return fmt.Errorf("feature quote %d %q: %w", index, quote, err)
		}
		for _, prior := range accepted {
			if span.Overlaps(prior) {
				return fmt.Errorf("feature quote %d %q overlaps another feature quote", index, quote)
			}
		}
		if len(accepted) > 0 && span.Start < accepted[len(accepted)-1].Start {
			return fmt.Errorf("feature quotes must preserve source order")
		}
		accepted = append(accepted, span)
	}
	return nil
}

func (classification ApplicationClassification) Validate() error {
	if classification.Schema != ApplicationClassificationSchemaV1 {
		return fmt.Errorf("application classification schema must be %q", ApplicationClassificationSchemaV1)
	}
	switch classification.Surface {
	case ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine,
		ApplicationSurfaceService, ApplicationSurfaceUnsupported:
		return nil
	default:
		return fmt.Errorf("application surface %q is unsupported", classification.Surface)
	}
}

func (identity ApplicationIdentity) ValidateFor(input ApplicationIdentityInput) error {
	if identity.Schema != ApplicationIdentitySchemaV1 {
		return fmt.Errorf("application identity schema must be %q", ApplicationIdentitySchemaV1)
	}
	if err := validateRequirementQuote("application product", identity.ProductQuote); err != nil {
		return err
	}
	if len(identity.ProductQuote) > maxApplicationProductBytes {
		return fmt.Errorf("application product quote exceeds %d bytes", maxApplicationProductBytes)
	}
	if _, err := uniqueTextSpan(input.UserRequest, identity.ProductQuote); err != nil {
		return fmt.Errorf("application product quote %q: %w", identity.ProductQuote, err)
	}
	return nil
}

func (specification ApplicationSpecification) Validate() error {
	switch specification.Surface {
	case ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine, ApplicationSurfaceService:
	case ApplicationSurfaceUnsupported:
		return fmt.Errorf("unsupported surface cannot be compiled")
	default:
		return fmt.Errorf("application surface %q is unsupported", specification.Surface)
	}
	if err := validateRequirementQuote("application product", specification.ProductQuote); err != nil {
		return err
	}
	if len(specification.ProductQuote) > maxApplicationProductBytes {
		return fmt.Errorf("application product quote exceeds %d bytes", maxApplicationProductBytes)
	}
	if len(specification.Requirements) == 0 {
		return fmt.Errorf("application specification requires at least one grounded requirement")
	}
	for index, requirement := range specification.Requirements {
		if requirement.ID != fmt.Sprintf("requirement_%03d", index+1) {
			return fmt.Errorf("requirement %d has non-code-owned identity %q", index, requirement.ID)
		}
		if err := validateRequirementQuote("application requirement", requirement.SourceQuote); err != nil {
			return fmt.Errorf("requirement %s: %w", requirement.ID, err)
		}
	}
	for index, artifact := range specification.Artifacts {
		if strings.TrimSpace(artifact.Token) == "" || strings.TrimSpace(string(artifact.Disposition)) == "" {
			return fmt.Errorf("artifact directive %d is incomplete", index)
		}
	}
	return nil
}
