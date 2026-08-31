package assemblyline

import (
	"fmt"
	"strings"
)

const ApplicationClassificationSchemaV2 = "omnidex.application-class.v2"

const (
	ArtifactHandlingSchemaV1   = "omnidex.artifact-handling.v1"
	maxApplicationProductBytes = 512
)

type ApplicationSurface string

const (
	ApplicationSurfaceBrowser     ApplicationSurface = "browser_application"
	ApplicationSurfaceCommandLine ApplicationSurface = "command_line_application"
	ApplicationSurfaceUnspecified ApplicationSurface = "unspecified"
	ApplicationSurfaceUnsupported ApplicationSurface = "unsupported"
	ApplicationDefaultSurface     ApplicationSurface = ApplicationSurfaceBrowser
)

type ApplicationClassification struct {
	Schema  string             `json:"schema"`
	Surface ApplicationSurface `json:"surface"`
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

func validateApplicationProductQuote(label, value string) error {
	return validateApplicationIntentText(label, value, maxApplicationProductBytes)
}

func (classification ApplicationClassification) Validate() error {
	if classification.Schema != ApplicationClassificationSchemaV2 {
		return fmt.Errorf("application classification schema must be %q", ApplicationClassificationSchemaV2)
	}
	switch classification.Surface {
	case ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine,
		ApplicationSurfaceUnspecified, ApplicationSurfaceUnsupported:
		return nil
	default:
		return fmt.Errorf("application surface %q is unsupported", classification.Surface)
	}
}

// ResolveApplicationSurface converts one validated semantic observation into
// the code-owned delivery authority used by stack selection. A request that
// does not constrain delivery receives the one registered default;
// an explicitly unsupported surface never falls through to that default.
func ResolveApplicationSurface(
	classification ApplicationClassification,
) (ApplicationSurface, error) {
	if err := classification.Validate(); err != nil {
		return "", err
	}
	switch classification.Surface {
	case ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine:
		return classification.Surface, nil
	case ApplicationSurfaceUnspecified:
		return ApplicationDefaultSurface, nil
	case ApplicationSurfaceUnsupported:
		return "", fmt.Errorf("software request requires an unsupported delivery surface")
	default:
		return "", fmt.Errorf("application surface %q is unsupported", classification.Surface)
	}
}

func (specification ApplicationSpecification) Validate() error {
	switch specification.Surface {
	case ApplicationSurfaceBrowser, ApplicationSurfaceCommandLine:
	case ApplicationSurfaceUnspecified:
		return fmt.Errorf("unspecified surface must be resolved before compilation")
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
	if len(specification.Requirements) < 1 || len(specification.Requirements) > maxRequirementCount {
		return fmt.Errorf(
			"application specification requires 1..%d grounded requirements",
			maxRequirementCount,
		)
	}
	seenRequirements := make(map[string]struct{}, len(specification.Requirements))
	for index, requirement := range specification.Requirements {
		if requirement.ID != fmt.Sprintf("requirement_%03d", index+1) {
			return fmt.Errorf("requirement %d has non-code-owned identity %q", index, requirement.ID)
		}
		if err := validateRequirementQuote("application requirement", requirement.SourceQuote); err != nil {
			return fmt.Errorf("requirement %s: %w", requirement.ID, err)
		}
		if _, duplicate := seenRequirements[requirement.SourceQuote]; duplicate {
			return fmt.Errorf("application requirement quote %q is duplicated", requirement.SourceQuote)
		}
		seenRequirements[requirement.SourceQuote] = struct{}{}
	}
	for index, artifact := range specification.Artifacts {
		if strings.TrimSpace(artifact.Token) == "" || strings.TrimSpace(string(artifact.Disposition)) == "" {
			return fmt.Errorf("artifact directive %d is incomplete", index)
		}
		switch artifact.Disposition {
		case ArtifactProtect, ArtifactRequire, ArtifactReference, ArtifactForbid,
			ArtifactAbsenceCandidate:
		default:
			return fmt.Errorf("artifact directive %d has unsupported disposition %q", index, artifact.Disposition)
		}
	}
	return nil
}
