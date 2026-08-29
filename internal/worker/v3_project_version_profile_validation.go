package worker

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

func directCodingVersionComponent(
	profile directCodingProjectVersionProfile,
	name string,
) (string, error) {
	for _, component := range profile.Components {
		if component.Name == name {
			return component.Version, nil
		}
	}
	return "", fmt.Errorf("version profile %s lacks component %s", profile.ID, name)
}

func matchNoManifestVersionProfile(
	directCodingProjectVersionProfile,
	map[string]string,
) (directCodingVersionCompatibility, error) {
	return directCodingVersionNotApplicable, nil
}

func matchGoVersionProfile(
	profile directCodingProjectVersionProfile,
	manifests map[string]string,
) (directCodingVersionCompatibility, error) {
	source, exists := manifests["go.mod"]
	if !exists {
		return directCodingVersionNotApplicable, nil
	}
	manifest, err := modfile.Parse("go.mod", []byte(source), nil)
	if err != nil {
		return directCodingVersionUnsupported, fmt.Errorf("parse existing go.mod version authority: %w", err)
	}
	constraint, err := directCodingVersionComponent(profile, "go_manifest")
	if err != nil {
		return directCodingVersionUnsupported, err
	}
	compatible := false
	if manifest.Go != nil {
		compatible, err = versionSatisfiesConstraint(manifest.Go.Version, constraint)
	}
	if err != nil {
		return directCodingVersionUnsupported, err
	}
	if !compatible {
		return directCodingVersionUnsupported, nil
	}
	return directCodingVersionCompatible, nil
}

func matchTypeScriptBrowserVersionProfile(
	profile directCodingProjectVersionProfile,
	manifests map[string]string,
) (directCodingVersionCompatibility, error) {
	source, exists := manifests["package.json"]
	if !exists {
		return directCodingVersionNotApplicable, nil
	}
	var syntax any
	if err := json.Unmarshal([]byte(source), &syntax); err != nil {
		return directCodingVersionUnsupported, fmt.Errorf(
			"decode existing TypeScript package manifest: %w", err,
		)
	}
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return directCodingVersionUnsupported, err
	}
	npm, err := directCodingVersionComponent(profile, "npm")
	if err != nil {
		return directCodingVersionUnsupported, err
	}
	if err := validateNPMManifestVersionProfile(
		source, profile, map[string]string{"node": node, "npm": npm},
	); err != nil {
		return directCodingVersionUnsupported, nil
	}
	return directCodingVersionCompatible, nil
}

func matchJavaScriptVersionProfile(
	profile directCodingProjectVersionProfile,
	manifests map[string]string,
) (directCodingVersionCompatibility, error) {
	source, exists := manifests["package.json"]
	if !exists {
		return directCodingVersionNotApplicable, nil
	}
	var manifest struct {
		Type    string            `json:"type"`
		Engines map[string]string `json:"engines"`
	}
	if err := json.Unmarshal([]byte(source), &manifest); err != nil {
		return directCodingVersionUnsupported, fmt.Errorf("decode existing JavaScript manifest: %w", err)
	}
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return directCodingVersionUnsupported, err
	}
	if manifest.Type != "module" || !reflect.DeepEqual(manifest.Engines, map[string]string{"node": node}) {
		return directCodingVersionUnsupported, nil
	}
	return directCodingVersionCompatible, nil
}

func matchRustVersionProfile(
	profile directCodingProjectVersionProfile,
	manifests map[string]string,
) (directCodingVersionCompatibility, error) {
	source, exists := manifests["Cargo.toml"]
	if !exists {
		return directCodingVersionNotApplicable, nil
	}
	var manifest struct {
		Package struct {
			Edition     string `toml:"edition"`
			RustVersion string `toml:"rust-version"`
		} `toml:"package"`
	}
	if err := toml.Unmarshal([]byte(source), &manifest); err != nil {
		return directCodingVersionUnsupported, fmt.Errorf("decode existing Cargo version authority: %w", err)
	}
	edition, err := directCodingVersionComponent(profile, "rust_edition")
	if err != nil {
		return directCodingVersionUnsupported, err
	}
	qualified, err := directCodingVersionComponent(profile, "rust_manifest")
	if err != nil {
		return directCodingVersionUnsupported, err
	}
	manifestVersion := normalizedSemver(manifest.Package.RustVersion)
	qualifiedVersion := normalizedSemver(qualified)
	if manifest.Package.Edition != edition || !semver.IsValid(manifestVersion) ||
		!semver.IsValid(qualifiedVersion) || semver.Compare(manifestVersion, qualifiedVersion) != 0 {
		return directCodingVersionUnsupported, nil
	}
	return directCodingVersionCompatible, nil
}

func matchPHPVersionProfile(
	profile directCodingProjectVersionProfile,
	manifests map[string]string,
) (directCodingVersionCompatibility, error) {
	source, exists := manifests["composer.json"]
	if !exists {
		return directCodingVersionNotApplicable, nil
	}
	var manifest struct {
		Require map[string]string `json:"require"`
	}
	if err := json.Unmarshal([]byte(source), &manifest); err != nil {
		return directCodingVersionUnsupported, fmt.Errorf("decode existing Composer version authority: %w", err)
	}
	constraint, err := directCodingVersionComponent(profile, "composer_php")
	if err != nil {
		return directCodingVersionUnsupported, err
	}
	if manifest.Require["php"] != constraint {
		return directCodingVersionUnsupported, nil
	}
	return directCodingVersionCompatible, nil
}

func versionAtLeast(value, minimum string) bool {
	value, minimum = normalizedSemver(value), normalizedSemver(minimum)
	return semver.IsValid(value) && semver.IsValid(minimum) && semver.Compare(value, minimum) >= 0
}

func normalizedSemver(value string) string {
	value = strings.TrimPrefix(strings.TrimSpace(value), "v")
	if strings.Count(value, ".") == 1 {
		value += ".0"
	}
	return "v" + value
}
