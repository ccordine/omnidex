package omni

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var projectSourceArtifactExtensions = map[string]struct{}{
	".js": {}, ".jsx": {}, ".ts": {}, ".tsx": {}, ".mjs": {}, ".cjs": {},
	".go": {}, ".rs": {}, ".zig": {}, ".css": {}, ".html": {}, ".vue": {},
	".py": {}, ".java": {}, ".kt": {}, ".swift": {}, ".cs": {},
}

func touchTargetsProjectSourceArtifact(command string) bool {
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) == 0 || cleanCommandPathToken(segment[0]) != "touch" {
			continue
		}
		for _, arg := range segment[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			if isProjectSourceArtifactPath(arg) {
				return true
			}
		}
	}
	return false
}

func isProjectSourceArtifactPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if _, ok := projectSourceArtifactExtensions[ext]; ok {
		return true
	}
	base := strings.ToLower(filepath.Base(path))
	return base == "dockerfile" || base == "makefile"
}

func validatePlaceholderOnlySourceMutation(command string, objectiveLedger []StructuredObjective, observations []StructuredCommandObservation) error {
	if !shellProposalIsPlaceholderOnlyMutation(command) {
		return nil
	}
	if touchTargetsProjectSourceArtifact(command) {
		return fmt.Errorf("placeholder-only touch creates empty source files; write substantive content with a here-doc, tee, patch.apply, or code specialist instead of touch")
	}
	if objectiveLedgerNeedsSubstantiveAppFiles(objectiveLedger) || workspaceMutationNeedsSubstantiveFiles(objectiveLedger, observations) {
		return fmt.Errorf("placeholder-only command does not satisfy app objectives; write substantive source/build/test file content instead of touch/mkdir alone")
	}
	return validateRepeatedPlaceholderOnlyAppMutation(command, observations, objectiveLedger)
}

func workspaceMutationNeedsSubstantiveFiles(objectiveLedger []StructuredObjective, observations []StructuredCommandObservation) bool {
	if objectiveLedgerNeedsSubstantiveAppFiles(objectiveLedger) {
		return true
	}
	for _, obs := range observations {
		if obs.ExitCode == 0 && structuredCommandLooksAppFileMutation(obs.Command) {
			return true
		}
	}
	return false
}

func classifyPlaceholderOnlyMutationAsFailure(command, workingDirectory string, exitCode int) (int, string) {
	if exitCode != 0 || !shellProposalIsPlaceholderOnlyMutation(command) {
		return exitCode, ""
	}
	if touchTargetsProjectSourceArtifact(command) {
		return 1, "partial_failure: placeholder-only touch left an empty source file; write substantive file content before advancing"
	}
	if workingDirectory != "" && placeholderMutationLeftEmptyArtifacts(command, workingDirectory) {
		return 1, "partial_failure: placeholder-only mutation left empty project files; fill or remove them before advancing"
	}
	return exitCode, ""
}

func placeholderMutationLeftEmptyArtifacts(command, workingDirectory string) bool {
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) == 0 {
			continue
		}
		root := cleanCommandPathToken(segment[0])
		if root != "touch" {
			continue
		}
		for _, arg := range segment[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			target := filepath.Join(workingDirectory, filepath.FromSlash(arg))
			if emptyProjectFileExists(target) {
				return true
			}
		}
	}
	return false
}

func validateConflictingEntrypointMutation(command, workingDirectory string) error {
	targets := mutationWriteTargetPaths(command)
	if len(targets) == 0 {
		return nil
	}
	for _, target := range targets {
		if !strings.EqualFold(filepath.Base(target), "index.html") {
			continue
		}
		if err := conflictingIndexHTMLShell(workingDirectory, target); err != nil {
			return err
		}
	}
	return nil
}

func mutationWriteTargetPaths(command string) []string {
	out := []string{}
	lower := strings.ToLower(command)
	if idx := strings.Index(lower, ">"); idx >= 0 {
		left := strings.TrimSpace(command[:idx])
		fields := strings.Fields(left)
		if len(fields) > 0 {
			out = append(out, fields[len(fields)-1])
		}
	}
	for _, segment := range structuredCommandSegments(command) {
		if len(segment) == 0 {
			continue
		}
		if cleanCommandPathToken(segment[0]) != "touch" {
			continue
		}
		for _, arg := range segment[1:] {
			if !strings.HasPrefix(arg, "-") {
				out = append(out, arg)
			}
		}
	}
	return uniqueNonEmptyStrings(out)
}

func conflictingIndexHTMLShell(workingDirectory, target string) error {
	target = filepath.ToSlash(strings.TrimSpace(target))
	if target == "" {
		return nil
	}
	candidates := []string{"index.html", "public/index.html", "src/index.html"}
	existing := []string{}
	for _, candidate := range candidates {
		if strings.EqualFold(candidate, target) {
			continue
		}
		path := filepath.Join(workingDirectory, filepath.FromSlash(candidate))
		if _, err := os.Stat(path); err == nil {
			existing = append(existing, candidate)
		}
	}
	if len(existing) == 0 {
		return nil
	}
	return fmt.Errorf("duplicate html entrypoint: %q conflicts with existing shell(s) %s; update the existing index.html instead of creating another", target, strings.Join(existing, ", "))
}
