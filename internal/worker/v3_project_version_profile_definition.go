package worker

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

func validateTypeScriptBrowserVersionProfile(profile directCodingProjectVersionProfile) error {
	if err := validateProfileComponents(
		profile, "ecmascript", "node", "npm", "npm_lock", "react", "tailwindcss",
		"typescript", "vite",
	); err != nil {
		return err
	}
	if err := validateProfileNPMComponent(profile, false, "react", "react"); err != nil {
		return err
	}
	for _, binding := range [][2]string{
		{"typescript", "typescript"}, {"vite", "vite"},
		{"tailwindcss", "tailwindcss"},
	} {
		if err := validateProfileNPMComponent(profile, true, binding[0], binding[1]); err != nil {
			return err
		}
	}
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return err
	}
	npm, err := directCodingVersionComponent(profile, "npm")
	if err != nil {
		return err
	}
	if err := validateProfileVersionConstraint(profile, "node", node); err != nil {
		return err
	}
	if err := validateProfileVersionConstraint(profile, "npm", npm); err != nil {
		return err
	}
	if err := validateProfileExactNPMVersions(profile); err != nil {
		return err
	}
	typescript, err := directCodingVersionComponent(profile, "typescript")
	if err != nil {
		return err
	}
	ecmascript, err := directCodingVersionComponent(profile, "ecmascript")
	if err != nil {
		return err
	}
	if err := requireProfileSourceDialect(
		profile, fmt.Sprintf(
			"TypeScript %s with TSX react-jsx targeting ECMAScript %s",
			typescript, strings.TrimPrefix(ecmascript, "ES"),
		),
	); err != nil {
		return err
	}
	lockVersion, err := directCodingNPMLockVersion(profile)
	if err != nil {
		return err
	}
	_, err = materializePinnedNPMLock(
		profile.NPMLockTemplate, "version-profile-qualification", lockVersion,
		profile.NPMDependencies, profile.NPMDevDependencies,
		map[string]string{"node": node, "npm": npm},
	)
	return err
}

func validateGoVersionProfile(profile directCodingProjectVersionProfile) error {
	if err := validateProfileComponents(profile, "go", "go_manifest"); err != nil {
		return err
	}
	generated, err := directCodingVersionComponent(profile, "go")
	if err != nil {
		return err
	}
	if err := validateProfileExactSemanticVersion(profile, "go", generated); err != nil {
		return err
	}
	constraint, err := directCodingVersionComponent(profile, "go_manifest")
	if err != nil {
		return err
	}
	compatible, err := versionSatisfiesConstraint(generated, constraint)
	if err != nil {
		return fmt.Errorf("version profile %s go_manifest: %w", profile.ID, err)
	}
	if !compatible {
		return fmt.Errorf("version profile %s generated Go version does not satisfy go_manifest", profile.ID)
	}
	return requireProfileSourceDialect(profile, "Go "+generated)
}

func validateJavaScriptVersionProfile(profile directCodingProjectVersionProfile) error {
	if err := validateProfileComponents(profile, "ecmascript", "node"); err != nil {
		return err
	}
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return err
	}
	if err := validateProfileVersionConstraint(profile, "node", node); err != nil {
		return err
	}
	ecmascript, err := directCodingVersionComponent(profile, "ecmascript")
	if err != nil {
		return err
	}
	if err := requireProfileSourceDialect(
		profile, "ECMAScript "+strings.TrimPrefix(ecmascript, "ES")+" modules on Node.js "+node,
	); err != nil {
		return err
	}
	_, err = javaScriptNodeRuntimeGuard(node)
	return err
}

func validateRustVersionProfile(profile directCodingProjectVersionProfile) error {
	if err := validateProfileComponents(
		profile, "cargo_lock", "rust_edition", "rust_manifest", "rust_version",
	); err != nil {
		return err
	}
	lock, err := directCodingVersionComponent(profile, "cargo_lock")
	if err != nil {
		return err
	}
	if value, parseErr := strconv.Atoi(lock); parseErr != nil || value <= 0 {
		return fmt.Errorf("version profile %s has invalid Cargo lock format %q", profile.ID, lock)
	}
	edition, err := directCodingVersionComponent(profile, "rust_edition")
	if err != nil {
		return err
	}
	if value, parseErr := strconv.Atoi(edition); parseErr != nil || value <= 0 {
		return fmt.Errorf("version profile %s has invalid Rust edition %q", profile.ID, edition)
	}
	manifest, err := directCodingVersionComponent(profile, "rust_manifest")
	if err != nil {
		return err
	}
	runtime, err := directCodingVersionComponent(profile, "rust_version")
	if err != nil {
		return err
	}
	if err := validateProfileExactSemanticVersion(profile, "rust_manifest", manifest); err != nil {
		return err
	}
	if err := validateProfileExactSemanticVersion(profile, "rust_version", runtime); err != nil {
		return err
	}
	if semver.Compare(normalizedSemver(manifest), normalizedSemver(runtime)) != 0 {
		return fmt.Errorf("version profile %s Rust manifest and runtime versions differ", profile.ID)
	}
	return requireProfileSourceDialect(
		profile, "Rust "+edition+" edition with rust-version "+runtime,
	)
}

func validateJavaVersionProfile(profile directCodingProjectVersionProfile) error {
	if err := validateProfileComponents(profile, "java_release"); err != nil {
		return err
	}
	release, err := directCodingVersionComponent(profile, "java_release")
	if err != nil {
		return err
	}
	if value, parseErr := strconv.Atoi(release); parseErr != nil || value <= 0 {
		return fmt.Errorf("version profile %s has invalid Java release %q", profile.ID, release)
	}
	return requireProfileSourceDialect(
		profile, "Java "+release+" source and class-file API release",
	)
}

func directCodingNPMLockVersion(profile directCodingProjectVersionProfile) (int, error) {
	value, err := directCodingVersionComponent(profile, "npm_lock")
	if err != nil {
		return 0, err
	}
	version, err := strconv.Atoi(value)
	if err != nil || version <= 0 {
		return 0, fmt.Errorf("version profile %s has invalid npm lock format %q", profile.ID, value)
	}
	return version, nil
}

func validateProfileNPMComponent(
	profile directCodingProjectVersionProfile,
	development bool,
	componentName string,
	packageName string,
) error {
	expected, err := directCodingVersionComponent(profile, componentName)
	if err != nil {
		return err
	}
	dependencies := profile.NPMDependencies
	if development {
		dependencies = profile.NPMDevDependencies
	}
	if dependencies[packageName] != expected {
		return fmt.Errorf(
			"version profile %s package %s differs from component %s",
			profile.ID, packageName, componentName,
		)
	}
	return nil
}

func validateProfileComponents(profile directCodingProjectVersionProfile, names ...string) error {
	if len(profile.Components) != len(names) {
		return fmt.Errorf(
			"version profile %s component count=%d want=%d", profile.ID, len(profile.Components), len(names),
		)
	}
	for _, name := range names {
		if _, err := directCodingVersionComponent(profile, name); err != nil {
			return err
		}
	}
	return nil
}
