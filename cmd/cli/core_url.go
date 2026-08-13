package main

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/envfile"
)

var errCoreURLRequired = errors.New("CORE_URL is required")

type managedEnvironmentReader func(string) (map[string]string, error)

func resolveCoreURL(explicit, executable string, readFile managedEnvironmentReader) (string, error) {
	if value := strings.TrimSpace(explicit); value != "" {
		return normalizedCoreURL(value)
	}
	if readFile == nil {
		return "", fmt.Errorf("managed environment reader is not configured")
	}
	configPath, err := managedEnvironmentPath(executable)
	if err != nil {
		return "", err
	}
	values, err := readFile(configPath)
	if err != nil {
		return "", fmt.Errorf("managed CORE_URL is unavailable at %s: %w", configPath, err)
	}
	value, found := values["CORE_URL"]
	if !found {
		return "", fmt.Errorf("managed environment %s does not define CORE_URL", configPath)
	}
	return normalizedCoreURL(value)
}

func managedEnvironmentPath(executable string) (string, error) {
	value := strings.TrimSpace(executable)
	if value == "" {
		return "", fmt.Errorf("cannot resolve managed CORE_URL: executable path is empty")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve executable path: %w", err)
	}
	binDir := filepath.Dir(absolute)
	if filepath.Base(binDir) != "bin" {
		return "", fmt.Errorf("CORE_URL is unset and executable is not in a managed bin directory: %s", absolute)
	}
	return filepath.Join(filepath.Dir(binDir), ".env"), nil
}

func readManagedEnvironment(path string) (map[string]string, error) {
	return envfile.Read(path)
}

func validateCoreURL(raw string) error {
	_, err := normalizedCoreURL(raw)
	return err
}

func normalizedCoreURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errCoreURLRequired
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("CORE_URL must be an absolute http or https URL")
	}
	if parsed.User != nil {
		return "", fmt.Errorf("CORE_URL must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("CORE_URL must not contain a query or fragment")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}
