package worker

import (
	"fmt"
	"regexp"
)

var directCodingPinnedContainerImagePattern = regexp.MustCompile(
	`^[a-z0-9][a-z0-9._/-]*:[A-Za-z0-9._-]+@sha256:[0-9a-f]{64}$`,
)

func validatePHPVersionProfile(profile directCodingProjectVersionProfile) error {
	if err := validateProfileComponents(
		profile, "composer_image", "composer_php", "docker_compose", "docker_engine", "nginx_image", "node",
		"node_image", "npm_lock", "php_runtime", "postgres_image", "tailwindcss",
	); err != nil {
		return err
	}
	if err := validateProfileNPMComponent(profile, true, "tailwindcss", "tailwindcss"); err != nil {
		return err
	}
	for _, component := range []string{"composer_image", "node_image", "nginx_image", "postgres_image"} {
		image, err := directCodingVersionComponent(profile, component)
		if err != nil {
			return err
		}
		if !directCodingPinnedContainerImagePattern.MatchString(image) {
			return fmt.Errorf(
				"version profile %s component %s requires one digest-pinned container image",
				profile.ID, component,
			)
		}
	}
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return err
	}
	if err := validateProfileExactSemanticVersion(profile, "node", node); err != nil {
		return err
	}
	composer, err := directCodingVersionComponent(profile, "composer_php")
	if err != nil {
		return err
	}
	if err := validateProfileVersionConstraint(profile, "composer_php", composer); err != nil {
		return err
	}
	compose, err := directCodingVersionComponent(profile, "docker_compose")
	if err != nil {
		return err
	}
	if err := validateProfileExactSemanticVersion(profile, "docker_compose", compose); err != nil {
		return err
	}
	engine, err := directCodingVersionComponent(profile, "docker_engine")
	if err != nil {
		return err
	}
	if err := validateProfileExactSemanticVersion(profile, "docker_engine", engine); err != nil {
		return err
	}
	if err := validateProfileExactNPMVersions(profile); err != nil {
		return err
	}
	if _, err := phpRuntimeVersionAssertion(profile); err != nil {
		return err
	}
	phpRuntime, err := directCodingVersionComponent(profile, "php_runtime")
	if err != nil {
		return err
	}
	parts := phpRuntimeRangePattern.FindStringSubmatch(phpRuntime)
	if len(parts) != 4 {
		return fmt.Errorf("version profile %s has invalid PHP runtime range", profile.ID)
	}
	lower := parts[1] + "." + parts[2] + ".0"
	compatible, err := versionSatisfiesConstraint(lower, composer)
	if err != nil {
		return err
	}
	if !compatible {
		return fmt.Errorf(
			"version profile %s PHP runtime lower bound does not satisfy Composer PHP constraint",
			profile.ID,
		)
	}
	if err := requireProfileSourceDialect(profile, "PHP "+phpRuntime+" function syntax"); err != nil {
		return err
	}
	lockVersion, err := directCodingNPMLockVersion(profile)
	if err != nil {
		return err
	}
	_, err = materializePinnedNPMLock(
		profile.NPMLockTemplate, "version-profile-qualification", lockVersion, nil,
		profile.NPMDevDependencies,
		map[string]string{"node": node},
	)
	return err
}
