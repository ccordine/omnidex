package worker

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

func validateProfileVersionConstraint(
	profile directCodingProjectVersionProfile,
	component string,
	constraint string,
) error {
	if _, err := versionSatisfiesConstraint("1.0.0", constraint); err != nil {
		return fmt.Errorf("version profile %s component %s: %w", profile.ID, component, err)
	}
	return nil
}

func validateProfileExactSemanticVersion(
	profile directCodingProjectVersionProfile,
	component string,
	version string,
) error {
	if version == "" || version != strings.TrimSpace(version) || strings.HasPrefix(version, "v") ||
		strings.ContainsAny(version, " <>^|=,") || !semver.IsValid(normalizedSemver(version)) {
		return fmt.Errorf(
			"version profile %s component %s requires one exact semantic version",
			profile.ID, component,
		)
	}
	return nil
}

func validateProfileExactNPMVersions(profile directCodingProjectVersionProfile) error {
	for kind, dependencies := range map[string]map[string]string{
		"dependency": profile.NPMDependencies, "development dependency": profile.NPMDevDependencies,
	} {
		for name, version := range dependencies {
			if err := validateProfileExactSemanticVersion(profile, kind+" "+name, version); err != nil {
				return err
			}
		}
	}
	return nil
}

func requireProfileSourceDialect(
	profile directCodingProjectVersionProfile,
	expected string,
) error {
	if profile.SourceDialect != expected {
		return fmt.Errorf(
			"version profile %s source dialect %q differs from component-derived %q",
			profile.ID, profile.SourceDialect, expected,
		)
	}
	return nil
}
