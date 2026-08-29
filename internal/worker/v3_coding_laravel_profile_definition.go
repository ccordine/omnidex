package worker

import (
	"crypto/sha256"
	"fmt"
)

func validateLaravelVersionProfile(profile directCodingProjectVersionProfile) error {
	if err := validateProfileComponents(
		profile,
		"composer", "composer_image", "composer_lock_sha256", "docker_compose", "docker_engine",
		"laravel_framework", "laravel_skeleton", "nginx_image", "node", "node_image",
		"npm_lock", "php", "php_image", "postgres_image", "tailwindcss",
	); err != nil {
		return err
	}
	for _, component := range []string{
		"composer_image", "nginx_image", "node_image", "php_image", "postgres_image",
	} {
		image, err := directCodingVersionComponent(profile, component)
		if err != nil {
			return err
		}
		if !directCodingPinnedContainerImagePattern.MatchString(image) {
			return fmt.Errorf("Laravel profile component %s requires a digest-pinned image", component)
		}
	}
	for _, component := range []string{
		"composer", "laravel_framework", "laravel_skeleton", "node", "php", "tailwindcss",
	} {
		value, err := directCodingVersionComponent(profile, component)
		if err != nil {
			return err
		}
		if err := validateProfileExactSemanticVersion(profile, component, value); err != nil {
			return err
		}
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
	if err := requireProfileSourceDialect(profile, "PHP "+laravelPHPVersion+" function syntax"); err != nil {
		return err
	}
	if !mapsEqual(profile.ComposerDependencies, laravelComposerDependencies) ||
		len(profile.ComposerDevDependencies) != 0 {
		return fmt.Errorf("Laravel profile Composer dependencies differ from exact registered authority")
	}
	lockSHA, err := directCodingVersionComponent(profile, "composer_lock_sha256")
	if err != nil {
		return err
	}
	if fmt.Sprintf("%x", sha256.Sum256(profile.ComposerLockTemplate)) != lockSHA {
		return fmt.Errorf("Laravel profile Composer lock digest differs from its component authority")
	}
	if err := validateLaravelComposerLock(profile.ComposerLockTemplate); err != nil {
		return err
	}
	if err := validateProfileNPMComponent(profile, true, "tailwindcss", "tailwindcss"); err != nil {
		return err
	}
	if err := validateProfileExactNPMVersions(profile); err != nil {
		return err
	}
	lockVersion, err := directCodingNPMLockVersion(profile)
	if err != nil {
		return err
	}
	_, err = materializePinnedNPMLock(
		profile.NPMLockTemplate, "laravel-profile-qualification", lockVersion, nil,
		profile.NPMDevDependencies, map[string]string{"node": directCodingPHPNodeVersion},
	)
	return err
}

func mapsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
