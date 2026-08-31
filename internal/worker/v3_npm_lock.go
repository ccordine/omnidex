package worker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func materializePinnedNPMLock(
	template []byte,
	packageName string,
	engines map[string]string,
) (string, map[string]string, map[string]string, error) {
	if packageName == "" {
		return "", nil, nil, fmt.Errorf("npm package lock requires one normalized package name")
	}
	var lock map[string]any
	if err := json.Unmarshal(template, &lock); err != nil {
		return "", nil, nil, fmt.Errorf("decode code-owned npm package lock: %w", err)
	}
	packages, packagesOK := lock["packages"].(map[string]any)
	root, rootOK := packages[""].(map[string]any)
	lockVersion, versionOK := lock["lockfileVersion"].(float64)
	if !packagesOK || !rootOK || !versionOK || lockVersion <= 0 ||
		lockVersion != float64(int(lockVersion)) || len(packages) < 2 {
		return "", nil, nil, fmt.Errorf("code-owned npm package lock lacks exact package authority")
	}
	dependencies, err := npmLockRootDependencySet(root, "dependencies")
	if err != nil {
		return "", nil, nil, err
	}
	devDependencies, err := npmLockRootDependencySet(root, "devDependencies")
	if err != nil {
		return "", nil, nil, err
	}
	if err := validatePinnedNPMDirectPackages(packages, dependencies, devDependencies); err != nil {
		return "", nil, nil, err
	}
	root["engines"] = engines
	lock["name"] = packageName
	root["name"] = packageName
	encoded, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return "", nil, nil, fmt.Errorf("encode code-owned npm package lock: %w", err)
	}
	return string(encoded) + "\n", dependencies, devDependencies, nil
}

func npmLockRootDependencySet(root map[string]any, field string) (map[string]string, error) {
	raw, exists := root[field]
	if !exists {
		return map[string]string{}, nil
	}
	values, valid := raw.(map[string]any)
	if !valid {
		return nil, fmt.Errorf("code-owned npm package lock root %s is not an object", field)
	}
	result := make(map[string]string, len(values))
	for name, candidate := range values {
		version, valid := candidate.(string)
		if !valid || name == "" || version == "" {
			return nil, fmt.Errorf("code-owned npm package lock root %s has invalid dependency authority", field)
		}
		result[name] = version
	}
	return result, nil
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
