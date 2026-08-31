package worker

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

func directCodingToolchainVersionCommand(program string) directCodingVerificationCommand {
	command := directCodingVerificationCommand{
		Argv: []string{program, "--version"}, Timeout: 15 * time.Second,
	}
	if program == "npm" {
		command.Environment = []string{"NPM_CONFIG_USERCONFIG=/dev/null"}
	}
	return command
}

func validateDirectCodingToolchainVersion(
	profile directCodingProjectVersionProfile,
	componentName string,
	output []byte,
) error {
	constraint, err := directCodingVersionComponent(profile, componentName)
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(output))
	if componentName == "node" {
		version = strings.TrimPrefix(version, "v")
	}
	canonical, err := directCodingCanonicalSemanticVersion(version)
	if err != nil {
		return fmt.Errorf("%s version output %q: %w", componentName, version, err)
	}
	accepted, err := directCodingSemanticVersionSatisfies(canonical, constraint)
	if err != nil {
		return fmt.Errorf("validate registered %s constraint %q: %w", componentName, constraint, err)
	}
	if !accepted {
		return fmt.Errorf(
			"observed %s version %s is outside selected profile %s constraint %s",
			componentName, strings.TrimPrefix(canonical, "v"), profile.ID, constraint,
		)
	}
	return nil
}

func directCodingCanonicalSemanticVersion(value string) (string, error) {
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return "", fmt.Errorf("semantic version is empty or contains whitespace")
	}
	canonical := "v" + strings.TrimPrefix(value, "v")
	if !semver.IsValid(canonical) {
		return "", fmt.Errorf("semantic version is invalid")
	}
	return canonical, nil
}

func directCodingSemanticVersionSatisfies(version string, constraint string) (bool, error) {
	if !semver.IsValid(version) || strings.TrimSpace(constraint) == "" {
		return false, fmt.Errorf("semantic version or constraint is invalid")
	}
	for _, alternative := range strings.Split(constraint, "||") {
		comparators := strings.Fields(strings.TrimSpace(alternative))
		if len(comparators) == 0 {
			return false, fmt.Errorf("constraint contains an empty alternative")
		}
		accepted := true
		for _, comparator := range comparators {
			matches, err := directCodingSemanticVersionComparator(version, comparator)
			if err != nil {
				return false, err
			}
			accepted = accepted && matches
		}
		if accepted {
			return true, nil
		}
	}
	return false, nil
}

func directCodingSemanticVersionComparator(version string, comparator string) (bool, error) {
	for _, operator := range []string{">=", "<=", ">", "<", "=", "^"} {
		if !strings.HasPrefix(comparator, operator) {
			continue
		}
		bound, err := directCodingCanonicalSemanticVersion(strings.TrimPrefix(comparator, operator))
		if err != nil {
			return false, fmt.Errorf("invalid semantic-version comparator %q", comparator)
		}
		comparison := semver.Compare(version, bound)
		switch operator {
		case ">=":
			return comparison >= 0, nil
		case "<=":
			return comparison <= 0, nil
		case ">":
			return comparison > 0, nil
		case "<":
			return comparison < 0, nil
		case "=":
			return comparison == 0, nil
		case "^":
			upper, err := directCodingSemanticVersionCaretUpper(bound)
			if err != nil {
				return false, err
			}
			return comparison >= 0 && semver.Compare(version, upper) < 0, nil
		}
	}
	bound, err := directCodingCanonicalSemanticVersion(comparator)
	if err != nil {
		return false, fmt.Errorf("invalid semantic-version comparator %q", comparator)
	}
	return semver.Compare(version, bound) == 0, nil
}

func directCodingSemanticVersionCaretUpper(version string) (string, error) {
	core := strings.TrimPrefix(semver.Canonical(version), "v")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("caret constraint requires a complete semantic version")
	}
	values := make([]int, 3)
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return "", fmt.Errorf("caret constraint contains an invalid numeric component")
		}
		values[index] = value
	}
	switch {
	case values[0] > 0:
		values[0]++
		values[1], values[2] = 0, 0
	case values[1] > 0:
		values[1]++
		values[2] = 0
	default:
		values[2]++
	}
	return fmt.Sprintf("v%d.%d.%d", values[0], values[1], values[2]), nil
}
