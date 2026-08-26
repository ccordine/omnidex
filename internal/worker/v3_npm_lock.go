package worker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

func materializePinnedNPMLock(
	template []byte,
	packageName string,
	expectedLockVersion int,
	dependencies map[string]string,
	devDependencies map[string]string,
	engines ...map[string]string,
) (string, error) {
	if packageName == "" || expectedLockVersion <= 0 {
		return "", fmt.Errorf("npm package lock requires one normalized package name")
	}
	var lock map[string]any
	if err := json.Unmarshal(template, &lock); err != nil {
		return "", fmt.Errorf("decode code-owned npm package lock: %w", err)
	}
	packages, packagesOK := lock["packages"].(map[string]any)
	root, rootOK := packages[""].(map[string]any)
	lockVersion, versionOK := lock["lockfileVersion"].(float64)
	if !packagesOK || !rootOK || !versionOK || lockVersion != float64(expectedLockVersion) || len(packages) < 2 {
		return "", fmt.Errorf(
			"code-owned npm package lock lacks format %d package authority", expectedLockVersion,
		)
	}
	if err := validatePinnedNPMDependencySet(root, "dependencies", dependencies); err != nil {
		return "", err
	}
	if err := validatePinnedNPMDependencySet(root, "devDependencies", devDependencies); err != nil {
		return "", err
	}
	if err := validatePinnedNPMDirectPackages(packages, dependencies, devDependencies); err != nil {
		return "", err
	}
	if len(engines) > 1 {
		return "", fmt.Errorf("npm package lock accepts at most one engine authority")
	}
	if len(engines) == 1 {
		root["engines"] = engines[0]
	}
	lock["name"] = packageName
	root["name"] = packageName
	encoded, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode code-owned npm package lock: %w", err)
	}
	return string(encoded) + "\n", nil
}

func validatePinnedNPMLockForManifest(manifestSource, lockSource string) error {
	var manifest struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Engines         map[string]string `json:"engines"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(manifestSource), &manifest); err != nil {
		return fmt.Errorf("decode npm package manifest: %w", err)
	}
	var lock map[string]any
	if err := json.Unmarshal([]byte(lockSource), &lock); err != nil {
		return fmt.Errorf("decode npm package lock: %w", err)
	}
	packages, packagesOK := lock["packages"].(map[string]any)
	root, rootOK := packages[""].(map[string]any)
	lockVersion, versionOK := lock["lockfileVersion"].(float64)
	if !packagesOK || !rootOK || !versionOK || lockVersion <= 0 || lockVersion != float64(int(lockVersion)) ||
		lock["name"] != manifest.Name || root["name"] != manifest.Name {
		return fmt.Errorf("npm package lock identity differs from its manifest")
	}
	if manifest.Version != "" && (lock["version"] != manifest.Version || root["version"] != manifest.Version) {
		return fmt.Errorf("npm package lock version differs from its manifest")
	}
	for field, expected := range map[string]map[string]string{
		"dependencies": manifest.Dependencies, "devDependencies": manifest.DevDependencies,
	} {
		actual := npmLockStringMap(root[field])
		if !reflect.DeepEqual(actual, expected) && !(len(actual) == 0 && len(expected) == 0) {
			return fmt.Errorf("npm package lock root %s differs from its manifest", field)
		}
	}
	actualEngines := npmLockStringMap(root["engines"])
	if !reflect.DeepEqual(actualEngines, manifest.Engines) &&
		!(len(actualEngines) == 0 && len(manifest.Engines) == 0) {
		return fmt.Errorf("npm package lock root engines differ from its manifest")
	}
	if err := validatePinnedNPMDirectPackages(
		packages, manifest.Dependencies, manifest.DevDependencies,
	); err != nil {
		return err
	}
	return nil
}

func validatePinnedNPMLockForProfile(
	manifestSource string,
	lockSource string,
	profile directCodingProjectVersionProfile,
) error {
	if err := validatePinnedNPMLockForManifest(manifestSource, lockSource); err != nil {
		return err
	}
	expected, err := directCodingNPMLockVersion(profile)
	if err != nil {
		return err
	}
	var lock struct {
		LockfileVersion int `json:"lockfileVersion"`
	}
	if err := json.Unmarshal([]byte(lockSource), &lock); err != nil {
		return fmt.Errorf("decode npm package lock format: %w", err)
	}
	if lock.LockfileVersion != expected {
		return fmt.Errorf(
			"npm package lock format %d differs from selected profile format %d",
			lock.LockfileVersion, expected,
		)
	}
	return nil
}

func npmLockStringMap(value any) map[string]string {
	raw, valid := value.(map[string]any)
	if !valid {
		return nil
	}
	values := make(map[string]string, len(raw))
	for name, candidate := range raw {
		version, valid := candidate.(string)
		if !valid {
			return nil
		}
		values[name] = version
	}
	return values
}

func validatePinnedNPMDependencySet(
	root map[string]any,
	field string,
	expected map[string]string,
) error {
	actual, exists := root[field].(map[string]any)
	if len(expected) == 0 {
		if !exists || len(actual) == 0 {
			return nil
		}
		return fmt.Errorf("code-owned npm package lock has unexpected root %s", field)
	}
	if !exists || len(actual) != len(expected) {
		return fmt.Errorf("code-owned npm package lock differs from pinned root %s", field)
	}
	for name, version := range expected {
		if actual[name] != version {
			return fmt.Errorf(
				"code-owned npm package lock %s %s differs from pinned version %s",
				field, name, version,
			)
		}
	}
	return nil
}

func validatePinnedNPMDirectPackages(
	packages map[string]any,
	dependencySets ...map[string]string,
) error {
	for _, dependencies := range dependencySets {
		for name, expectedVersion := range dependencies {
			entry, exists := packages["node_modules/"+name].(map[string]any)
			if !exists || entry["version"] != expectedVersion {
				return fmt.Errorf(
					"code-owned npm package lock lacks exact direct package %s %s",
					name, expectedVersion,
				)
			}
			integrity, valid := entry["integrity"].(string)
			if !valid || !validSHA512Integrity(integrity) {
				return fmt.Errorf(
					"code-owned npm package lock direct package %s lacks sha512 integrity", name,
				)
			}
		}
	}
	return nil
}

func validSHA512Integrity(value string) bool {
	if !strings.HasPrefix(value, "sha512-") || strings.ContainsAny(value, " \t\r\n") {
		return false
	}
	digest, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "sha512-"))
	return err == nil && len(digest) == 64
}
