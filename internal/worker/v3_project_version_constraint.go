package worker

import (
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"
)

func versionSatisfiesConstraint(version, constraint string) (bool, error) {
	version = normalizedSemver(version)
	if !semver.IsValid(version) {
		return false, fmt.Errorf("version is not semantic: %q", version)
	}
	alternatives := strings.Split(constraint, "||")
	matched := false
	for _, raw := range alternatives {
		tokens := strings.Fields(strings.TrimSpace(raw))
		if len(tokens) == 0 {
			return false, fmt.Errorf("version constraint contains an empty alternative")
		}
		alternative := true
		for _, token := range tokens {
			accepted, err := versionSatisfiesConstraintToken(version, token)
			if err != nil {
				return false, err
			}
			alternative = alternative && accepted
		}
		matched = matched || alternative
	}
	return matched, nil
}

func versionSatisfiesConstraintToken(version, token string) (bool, error) {
	operator := "="
	wanted := token
	for _, candidate := range []string{">=", "<=", ">", "<", "=", "^"} {
		if strings.HasPrefix(token, candidate) {
			operator = candidate
			wanted = strings.TrimPrefix(token, candidate)
			break
		}
	}
	wanted = normalizedSemver(wanted)
	if !semver.IsValid(wanted) {
		return false, fmt.Errorf("version constraint token %q is invalid", token)
	}
	comparison := semver.Compare(version, wanted)
	switch operator {
	case "=":
		return comparison == 0, nil
	case ">=":
		return comparison >= 0, nil
	case "<=":
		return comparison <= 0, nil
	case ">":
		return comparison > 0, nil
	case "<":
		return comparison < 0, nil
	case "^":
		upper, err := caretConstraintUpperBound(wanted)
		if err != nil {
			return false, err
		}
		return comparison >= 0 && semver.Compare(version, upper) < 0, nil
	default:
		return false, fmt.Errorf("version constraint operator %q is unsupported", operator)
	}
}

func caretConstraintUpperBound(version string) (string, error) {
	core := strings.TrimPrefix(semver.Canonical(version), "v")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("caret constraint %q is not a three-part semantic version", version)
	}
	numbers := make([]int, 3)
	for index, part := range parts {
		value, err := strconv.Atoi(part)
		if err != nil {
			return "", fmt.Errorf("caret constraint %q is invalid", version)
		}
		numbers[index] = value
	}
	switch {
	case numbers[0] > 0:
		numbers[0]++
		numbers[1], numbers[2] = 0, 0
	case numbers[1] > 0:
		numbers[1]++
		numbers[2] = 0
	default:
		numbers[2]++
	}
	return fmt.Sprintf("v%d.%d.%d", numbers[0], numbers[1], numbers[2]), nil
}
