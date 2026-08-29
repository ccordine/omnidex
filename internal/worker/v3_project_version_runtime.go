package worker

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

type directCodingVersionProbe func(program string, args ...string) (string, error)

var directCodingRuntimeVersionGrammars = map[string]*regexp.Regexp{
	"node":   regexp.MustCompile(`^v([0-9]+\.[0-9]+\.[0-9]+)$`),
	"npm":    regexp.MustCompile(`^([0-9]+\.[0-9]+\.[0-9]+)$`),
	"go":     regexp.MustCompile(`^go version go([0-9]+\.[0-9]+(?:\.[0-9]+)?)(?:[-+][^ ]+)? [^ /\n]+/[^ /\n]+$`),
	"rustc":  regexp.MustCompile(`^rustc ([0-9]+\.[0-9]+\.[0-9]+) \([^)]+\)$`),
	"cargo":  regexp.MustCompile(`^cargo ([0-9]+\.[0-9]+\.[0-9]+) \([^)]+\)$`),
	"javac":  regexp.MustCompile(`^javac ([0-9]+(?:\.[0-9]+){0,2})(?:[-+][A-Za-z0-9._-]+)?$`),
	"jar":    regexp.MustCompile(`^jar ([0-9]+(?:\.[0-9]+){0,2})(?:[-+][A-Za-z0-9._-]+)?$`),
	"docker": regexp.MustCompile(`^([0-9]+\.[0-9]+\.[0-9]+)$`),
}

var directCodingJavaVersionGrammar = regexp.MustCompile(
	`^(?:openjdk|java) version "([0-9]+(?:\.[0-9]+){0,2})"[^\n]*\n[^\n]+\n[^\n]+$`,
)

func directCodingSessionVersionProbe(ctx context.Context, root string) directCodingVersionProbe {
	return func(program string, args ...string) (string, error) {
		execution, err := runValidatedV3Command(ctx, root, codeCommand{
			Program: program, Args: append([]string(nil), args...), Timeout: 5 * time.Second,
		})
		if err != nil {
			return "", fmt.Errorf("version probe %s is outside the command boundary: %w", program, err)
		}
		if execution.ContextError != nil {
			return "", fmt.Errorf("version probe %s exceeded its timeout: %w", program, execution.ContextError)
		}
		if execution.RunError != nil {
			return "", fmt.Errorf(
				"version probe %s exited %d: %w: %s", program, execution.ExitCode,
				execution.RunError, trimForBudget(renderV3CommandOutput(execution), 500),
			)
		}
		return renderV3CommandOutput(execution), nil
	}
}

func validateDirectCodingSelectedVersionRuntime(
	selection directCodingProjectSelection,
	probe directCodingVersionProbe,
) error {
	profile, err := directCodingProjectVersionProfileByID(selection.VersionProfileID)
	if err != nil {
		return err
	}
	if profile.StackID != selection.Stack.ID {
		return fmt.Errorf(
			"selected project stack %s is not qualified by version profile %s",
			selection.Stack.ID, profile.ID,
		)
	}
	return validateDirectCodingVersionProfileRuntime(profile, probe)
}

func validateDirectCodingVersionProfileRuntime(
	profile directCodingProjectVersionProfile,
	probe directCodingVersionProbe,
) error {
	if probe == nil {
		return fmt.Errorf("selected project version requires one bounded runtime probe")
	}
	if profile.ValidateRuntime == nil {
		return fmt.Errorf("project version profile %s has no runtime qualification", profile.ID)
	}
	if err := profile.ValidateRuntime(profile, probe); err != nil {
		return fmt.Errorf("qualify runtime for project version profile %s: %w", profile.ID, err)
	}
	return nil
}

func validateTypeScriptBrowserRuntimeProfile(
	profile directCodingProjectVersionProfile,
	probe directCodingVersionProbe,
) error {
	node, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return err
	}
	npm, err := directCodingVersionComponent(profile, "npm")
	if err != nil {
		return err
	}
	if err := validateRuntimeCommandConstraint(probe, "node", []string{"--version"}, node); err != nil {
		return err
	}
	return validateRuntimeCommandConstraint(probe, "npm", []string{"--version"}, npm)
}

func validateGoRuntimeProfile(profile directCodingProjectVersionProfile, probe directCodingVersionProbe) error {
	minimum, err := directCodingVersionComponent(profile, "go")
	if err != nil {
		return err
	}
	return validateRuntimeCommandMinimum(probe, "go", []string{"version"}, minimum)
}

func validateJavaScriptRuntimeProfile(profile directCodingProjectVersionProfile, probe directCodingVersionProbe) error {
	constraint, err := directCodingVersionComponent(profile, "node")
	if err != nil {
		return err
	}
	return validateRuntimeCommandConstraint(probe, "node", []string{"--version"}, constraint)
}

func validateRustRuntimeProfile(profile directCodingProjectVersionProfile, probe directCodingVersionProbe) error {
	minimum, err := directCodingVersionComponent(profile, "rust_version")
	if err != nil {
		return err
	}
	if err := validateRuntimeCommandMinimum(probe, "rustc", []string{"--version"}, minimum); err != nil {
		return err
	}
	return validateRuntimeCommandMinimum(probe, "cargo", []string{"--version"}, minimum)
}

func validateJavaRuntimeProfile(profile directCodingProjectVersionProfile, probe directCodingVersionProbe) error {
	release, err := directCodingVersionComponent(profile, "java_release")
	if err != nil {
		return err
	}
	minimum := release + ".0.0"
	if err := validateRuntimeCommandMinimum(
		probe, "javac", []string{"--release", release, "-version"}, minimum,
	); err != nil {
		return err
	}
	if err := validateRuntimeCommandMinimum(probe, "java", []string{"-version"}, minimum); err != nil {
		return err
	}
	return validateRuntimeCommandMinimum(probe, "jar", []string{"--version"}, minimum)
}

func validatePHPRuntimeProfile(profile directCodingProjectVersionProfile, probe directCodingVersionProbe) error {
	engine, err := directCodingVersionComponent(profile, "docker_engine")
	if err != nil {
		return err
	}
	if err := validateRuntimeCommandConstraint(
		probe, "docker", []string{"version", "--format", "{{.Server.Version}}"}, engine,
	); err != nil {
		return err
	}
	compose, err := directCodingVersionComponent(profile, "docker_compose")
	if err != nil {
		return err
	}
	return validateRuntimeCommandConstraint(
		probe, "docker", []string{"compose", "version", "--short"}, compose,
	)
}

func validateRuntimeCommandMinimum(
	probe directCodingVersionProbe,
	program string,
	args []string,
	minimum string,
) error {
	version, err := directCodingRuntimeCommandVersion(probe, program, args...)
	if err != nil {
		return err
	}
	if !versionAtLeast(version, minimum) {
		return fmt.Errorf("%s runtime %s is below qualified minimum %s", program, version, minimum)
	}
	return nil
}

func validateRuntimeCommandConstraint(
	probe directCodingVersionProbe,
	program string,
	args []string,
	constraint string,
) error {
	version, err := directCodingRuntimeCommandVersion(probe, program, args...)
	if err != nil {
		return err
	}
	compatible, err := versionSatisfiesConstraint(version, constraint)
	if err != nil {
		return fmt.Errorf("validate %s constraint %q: %w", program, constraint, err)
	}
	if !compatible {
		return fmt.Errorf("%s runtime %s does not satisfy qualified constraint %s", program, version, constraint)
	}
	return nil
}

func directCodingRuntimeCommandVersion(
	probe directCodingVersionProbe,
	program string,
	args ...string,
) (string, error) {
	output, err := probe(program, args...)
	if err != nil {
		return "", err
	}
	output = strings.TrimSpace(output)
	grammar := directCodingRuntimeVersionGrammars[program]
	if program == "java" {
		grammar = directCodingJavaVersionGrammar
	}
	if grammar == nil {
		return "", fmt.Errorf("required runtime %s has no registered version grammar", program)
	}
	match := grammar.FindStringSubmatch(output)
	if len(match) != 2 {
		return "", fmt.Errorf("required runtime %s returned output outside its exact version grammar", program)
	}
	return match[1], nil
}

func versionSatisfiesConstraint(version, constraint string) (bool, error) {
	version = normalizedSemver(version)
	if !semver.IsValid(version) {
		return false, fmt.Errorf("version %q is not semantic", version)
	}
	matchedAlternative := false
	for _, alternative := range strings.Split(constraint, "||") {
		tokens := strings.Fields(strings.TrimSpace(alternative))
		if len(tokens) == 0 {
			return false, fmt.Errorf("constraint %q has an empty alternative", constraint)
		}
		matches := true
		for _, token := range tokens {
			matched, err := versionMatchesComparator(version, token)
			if err != nil {
				return false, err
			}
			matches = matches && matched
		}
		matchedAlternative = matchedAlternative || matches
	}
	return matchedAlternative, nil
}

func versionMatchesComparator(version, comparator string) (bool, error) {
	operator, candidate := "=", comparator
	for _, prefix := range []string{">=", "<=", ">", "<", "^"} {
		if strings.HasPrefix(comparator, prefix) {
			operator, candidate = prefix, strings.TrimPrefix(comparator, prefix)
			break
		}
	}
	candidate = normalizedSemver(candidate)
	if !semver.IsValid(candidate) {
		return false, fmt.Errorf("comparator %q is unsupported", comparator)
	}
	comparison := semver.Compare(version, candidate)
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
		major, err := strconv.Atoi(strings.Split(strings.TrimPrefix(candidate, "v"), ".")[0])
		if err != nil || major == 0 {
			return false, fmt.Errorf("caret comparator %q requires a positive major", comparator)
		}
		upper := fmt.Sprintf("v%d.0.0", major+1)
		return comparison >= 0 && semver.Compare(version, upper) < 0, nil
	default:
		return false, fmt.Errorf("comparator %q is unsupported", comparator)
	}
}
