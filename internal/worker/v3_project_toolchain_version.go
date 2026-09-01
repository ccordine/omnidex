package worker

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

func directCodingToolchainVersionCommand(program string) directCodingVerificationCommand {
	if program == "go" {
		return directCodingGoVersionCommand()
	}
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
	if componentName == "go" {
		return validateDirectCodingGoToolchainVersion(profile, output)
	}
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

func directCodingGoVersionCommand() directCodingVerificationCommand {
	return directCodingVerificationCommand{
		Argv: []string{"go", "version"}, Timeout: 15 * time.Second,
	}
}

func validateDirectCodingGoToolchainVersion(
	profile directCodingProjectVersionProfile,
	output []byte,
) error {
	minimum, err := directCodingVersionComponent(profile, "go")
	if err != nil {
		return err
	}
	minimumVersion, err := directCodingCanonicalSemanticVersion(minimum)
	if err != nil {
		return fmt.Errorf("registered Go source version %q: %w", minimum, err)
	}
	observed, err := directCodingGoVersionFromOutput(output)
	if err != nil {
		return err
	}
	if semver.Compare(observed, minimumVersion) < 0 {
		return fmt.Errorf(
			"observed Go compiler version %s is older than selected profile %s source version %s",
			strings.TrimPrefix(observed, "v"), profile.ID, minimum,
		)
	}
	return nil
}

func directCodingGoVersionFromOutput(output []byte) (string, error) {
	line := strings.TrimSpace(string(output))
	if strings.ContainsAny(line, "\r\n") {
		return "", fmt.Errorf("Go version output %q contains more than one line", line)
	}
	fields := strings.Fields(line)
	if len(fields) != 4 || fields[0] != "go" || fields[1] != "version" {
		return "", fmt.Errorf("Go version output %q does not have the native four-field form", line)
	}
	if !directCodingGoPlatformIsValid(fields[3]) {
		return "", fmt.Errorf("Go version output %q has an invalid platform", line)
	}
	value := strings.TrimPrefix(fields[2], "go")
	if value == fields[2] {
		return "", fmt.Errorf("Go version output %q omits the go-prefixed release", line)
	}
	coreEnd := 0
	for coreEnd < len(value) && ((value[coreEnd] >= '0' && value[coreEnd] <= '9') || value[coreEnd] == '.') {
		coreEnd++
	}
	core, suffix := value[:coreEnd], value[coreEnd:]
	parts := strings.Split(core, ".")
	if len(parts) != 2 && len(parts) != 3 {
		return "", fmt.Errorf("Go version output %q has an invalid release", line)
	}
	for _, part := range parts {
		if part == "" {
			return "", fmt.Errorf("Go version output %q has an invalid release", line)
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return "", fmt.Errorf("Go version output %q has an invalid release", line)
			}
		}
	}
	if len(parts) == 2 {
		core += ".0"
	}
	canonical, err := directCodingCanonicalSemanticVersion(core)
	if err != nil {
		return "", fmt.Errorf("Go version output %q: %w", line, err)
	}
	if suffix == "" || directCodingGoVendorSuffixIsValid(suffix) {
		return canonical, nil
	}
	if prerelease, ok := directCodingGoPrereleaseSuffix(suffix); ok {
		return canonical + "-" + prerelease, nil
	}
	return "", fmt.Errorf("Go version output %q has an invalid release suffix", line)
}

func directCodingGoPlatformIsValid(value string) bool {
	osName, architecture, found := strings.Cut(value, "/")
	if !found || osName == "" || architecture == "" || strings.Contains(architecture, "/") {
		return false
	}
	for _, component := range []string{osName, architecture} {
		for _, character := range component {
			if (character < 'a' || character > 'z') &&
				(character < '0' || character > '9') && character != '_' {
				return false
			}
		}
	}
	return true
}

func directCodingGoVendorSuffixIsValid(value string) bool {
	if !strings.HasPrefix(value, "-") || len(value) == 1 {
		return false
	}
	for _, character := range value[1:] {
		if (character < 'a' || character > 'z') &&
			(character < 'A' || character > 'Z') &&
			(character < '0' || character > '9') &&
			!strings.ContainsRune("._:+-", character) {
			return false
		}
	}
	return true
}

func directCodingGoPrereleaseSuffix(value string) (string, bool) {
	for _, prefix := range []string{"beta", "rc"} {
		if !strings.HasPrefix(value, prefix) || len(value) == len(prefix) {
			continue
		}
		for _, character := range value[len(prefix):] {
			if character < '0' || character > '9' {
				return "", false
			}
		}
		return value, true
	}
	return "", false
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
