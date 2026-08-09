package worker

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxRepositoryGoCorrectionDiagnosticBytes = 1024

var (
	repositoryGoDiagnosticLocation = regexp.MustCompile(
		`^\s*(.+?\.go):(\d+)(?::\d+)?:\s*(\S.*)\s*$`,
	)
)

type repositoryGoVerificationFailure struct {
	targetSymbolID string
	diagnostic     string
}

func (failure *repositoryGoVerificationFailure) Error() string {
	if failure == nil {
		return "repository staged Go verification failure is absent"
	}
	return fmt.Sprintf(
		"repository staged Go verification uniquely owns target %q: %s",
		failure.targetSymbolID, failure.diagnostic,
	)
}

type repositoryGoFailureOutput struct {
	testName string
	line     string
}

func classifyRepositoryGoVerificationFailure(
	command testCommand,
	stdout string,
	ownership repositoryGoCorrectionOwnership,
) (*repositoryGoVerificationFailure, error) {
	if err := validateRepositoryGoTestCommand(command); err != nil {
		return nil, err
	}
	if len(ownership.targets) == 0 || ownership.stagedRoot == "" {
		return nil, fmt.Errorf("repository Go failure classification has no staged target ownership")
	}
	failedTests, outputs, err := parseRepositoryGoFailureOutput(stdout)
	if err != nil {
		return nil, err
	}
	targets := make(map[string]struct{})
	diagnostics := make(map[string][]string)
	for _, expected := range command.RepositoryProof.Expected {
		if _, failed := failedTests[expected.Name]; !failed {
			continue
		}
		for _, targetID := range expected.TargetSymbolIDs {
			targets[targetID] = struct{}{}
			for _, output := range outputs {
				if output.testName != expected.Name {
					continue
				}
				if diagnostic, ok := pathFreeRepositoryGoDiagnostic(output.line); ok {
					diagnostics[targetID] = appendUniqueSorted(diagnostics[targetID], diagnostic)
				}
			}
		}
	}
	for _, output := range outputs {
		path, line, diagnostic, ok := repositoryGoLocatedDiagnostic(output.line)
		if !ok {
			continue
		}
		relative, ok := normalizeRepositoryGoDiagnosticPath(ownership.stagedRoot, path)
		if !ok {
			continue
		}
		for _, owner := range ownership.targets {
			if owner.filePath != relative || line < owner.startLine || line > owner.endLine {
				continue
			}
			targets[owner.targetSymbolID] = struct{}{}
			diagnostics[owner.targetSymbolID] = appendUniqueSorted(
				diagnostics[owner.targetSymbolID], diagnostic,
			)
		}
	}
	owned := make([]string, 0, len(targets))
	for targetID := range targets {
		owned = append(owned, targetID)
	}
	sort.Strings(owned)
	if len(owned) == 0 {
		return nil, fmt.Errorf("repository staged Go verification failure has no exact contract target owner")
	}
	if len(owned) != 1 {
		return nil, fmt.Errorf(
			"repository staged Go verification failure implicates multiple contract targets: %s",
			strings.Join(owned, ", "),
		)
	}
	diagnosticSet := diagnostics[owned[0]]
	if len(diagnosticSet) == 0 {
		return nil, fmt.Errorf(
			"repository staged Go verification target %q has no bounded path-free diagnostic",
			owned[0],
		)
	}
	if len(diagnosticSet) != 1 {
		return nil, fmt.Errorf(
			"repository staged Go verification target %q has multiple exact diagnostics",
			owned[0],
		)
	}
	return &repositoryGoVerificationFailure{
		targetSymbolID: owned[0], diagnostic: diagnosticSet[0],
	}, nil
}

func parseRepositoryGoFailureOutput(
	stdout string,
) (map[string]struct{}, []repositoryGoFailureOutput, error) {
	if len([]byte(stdout)) > maxRepositoryGoVerificationStdoutBytes {
		return nil, nil, fmt.Errorf("repository failed Go proof exceeds its exact evidence bound")
	}
	failed := make(map[string]struct{})
	outputs := make([]repositoryGoFailureOutput, 0)
	failureEvent := false
	for index, line := range strings.Split(strings.ReplaceAll(stdout, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var event goTestEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, nil, fmt.Errorf("repository failed Go proof line %d is malformed JSON: %w", index+1, err)
		}
		if event.Action == "build-output" || event.Action == "build-fail" {
			if event.ImportPath == "" || event.Package != "" || event.Test != "" {
				return nil, nil, fmt.Errorf("repository failed Go proof line %d has invalid build authority", index+1)
			}
			if event.Action == "build-fail" {
				failureEvent = true
				continue
			}
			for _, outputLine := range strings.Split(strings.ReplaceAll(event.Output, "\r\n", "\n"), "\n") {
				if strings.TrimSpace(outputLine) != "" {
					outputs = append(outputs, repositoryGoFailureOutput{line: outputLine})
				}
			}
			continue
		}
		if event.Package == "" || !registeredGoTestAction(event.Action) {
			return nil, nil, fmt.Errorf("repository failed Go proof line %d has invalid structured authority", index+1)
		}
		if event.Action == "fail" {
			failureEvent = true
			if event.Test != "" {
				name, _, _ := strings.Cut(event.Test, "/")
				failed[name] = struct{}{}
			}
		}
		if event.Action != "output" || strings.TrimSpace(event.Output) == "" {
			continue
		}
		testName, _, _ := strings.Cut(event.Test, "/")
		for _, outputLine := range strings.Split(strings.ReplaceAll(event.Output, "\r\n", "\n"), "\n") {
			if strings.TrimSpace(outputLine) != "" {
				outputs = append(outputs, repositoryGoFailureOutput{testName: testName, line: outputLine})
			}
		}
	}
	if !failureEvent {
		return nil, nil, fmt.Errorf("repository failed Go proof contains no structured failure event")
	}
	return failed, outputs, nil
}

func repositoryGoLocatedDiagnostic(value string) (string, int, string, bool) {
	match := repositoryGoDiagnosticLocation.FindStringSubmatch(value)
	if len(match) != 4 {
		return "", 0, "", false
	}
	line, err := strconv.Atoi(match[2])
	if err != nil || line < 1 {
		return "", 0, "", false
	}
	diagnostic, ok := validateRepositoryGoPathFreeDiagnostic(match[3])
	return strings.TrimSpace(match[1]), line, diagnostic, ok
}

func pathFreeRepositoryGoDiagnostic(value string) (string, bool) {
	if _, _, diagnostic, ok := repositoryGoLocatedDiagnostic(value); ok {
		return diagnostic, true
	}
	return validateRepositoryGoPathFreeDiagnostic(value)
}

func validateRepositoryGoPathFreeDiagnostic(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') ||
		strings.ContainsAny(value, "\r\n\\") || len([]byte(value)) > maxRepositoryGoCorrectionDiagnosticBytes {
		return "", false
	}
	if containsModelContextPathIdentity(value) || strings.Contains(value, repositorySandboxRoot) {
		return "", false
	}
	if strings.HasPrefix(value, "=== ") || strings.HasPrefix(value, "--- ") || strings.HasPrefix(value, "# ") {
		return "", false
	}
	return value, true
}

func normalizeRepositoryGoDiagnosticPath(root, value string) (string, bool) {
	value = filepath.ToSlash(strings.TrimSpace(value))
	root = filepath.ToSlash(filepath.Clean(root))
	switch {
	case strings.HasPrefix(value, repositorySandboxRoot+"/"):
		value = strings.TrimPrefix(value, repositorySandboxRoot+"/")
	case strings.HasPrefix(value, root+"/"):
		value = strings.TrimPrefix(value, root+"/")
	case filepath.IsAbs(filepath.FromSlash(value)):
		return "", false
	}
	value = strings.TrimPrefix(value, "./")
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return cleaned, true
}
