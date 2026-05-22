package omni

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const emptyProjectFileObjectiveID = "resolve_empty_project_files"

func enforceNoEmptyProjectFilesBeforeCompletion(step int, prompt, workingDir string, ledger []StructuredObjective, observations []StructuredCommandObservation, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) []StructuredObjective {
	if !shouldScanEmptyProjectFiles(prompt, ledger, observations) {
		return ledger
	}
	files := findEmptyProjectFiles(workingDir, 12)
	if len(files) == 0 {
		return ledger
	}
	evidence := "empty project file(s) remain before completion: " + strings.Join(files, ",")
	emitStructuredCommandEvent(onEvent, "completion_check_rejected_empty_files", "Completion blocked by empty project files", map[string]string{
		"step":  fmt.Sprintf("%d", step),
		"files": strings.Join(files, ","),
	})
	if result != nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "completion blocked: " + evidence + "; fill each empty source/test/config file with substantive content or remove it if unused, then verify again",
		})
	}
	return mergeStructuredObjectiveLedger(ledger, []StructuredObjective{{
		ID:          emptyProjectFileObjectiveID,
		Description: "Fill or remove empty placeholder project files before completion",
		Status:      "pending",
		Evidence:    evidence,
		Source:      structuredObjectiveSourceEvidenceRequiredPrerequisite,
		Required:    true,
	}})
}

func shouldScanEmptyProjectFiles(prompt string, ledger []StructuredObjective, observations []StructuredCommandObservation) bool {
	if appBuildPromptNeedsFiles(prompt) || objectiveLedgerNeedsSubstantiveAppFiles(ledger) {
		return true
	}
	for _, obs := range observations {
		if obs.ExitCode == 0 && structuredCommandLooksMutating(obs.Command) {
			return true
		}
	}
	return false
}

func objectiveLedgerHasActiveEmptyFileCleanup(ledger []StructuredObjective) bool {
	for _, objective := range pendingStructuredObjectives(ledger) {
		text := strings.ToLower(objective.ID + " " + objective.Description)
		if strings.Contains(text, "remove_empty") ||
			strings.Contains(text, "empty placeholder") ||
			strings.Contains(text, "empty file") ||
			strings.Contains(text, "cleanup placeholder") ||
			strings.Contains(text, "clean up placeholder") {
			return true
		}
	}
	return false
}

func findEmptyProjectFiles(root string, limit int) []string {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil
	}
	if limit <= 0 {
		limit = 12
	}
	out := []string{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || len(out) >= limit {
			return nil
		}
		name := entry.Name()
		if entry.IsDir() {
			if shouldSkipEmptyFileScanDir(name) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !emptyFileGuardRelevant(path, name) {
			return nil
		}
		info, statErr := entry.Info()
		if statErr != nil || info.Size() != 0 {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	return out
}

func shouldSkipEmptyFileScanDir(name string) bool {
	switch name {
	case ".git", ".omni", "node_modules", "vendor", "dist", "build", "target", "coverage", ".next", ".vite":
		return true
	default:
		return strings.HasPrefix(name, ".cache")
	}
}

func emptyFileGuardRelevant(path, name string) bool {
	if strings.HasSuffix(name, ".lock") || name == "go.sum" {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go", ".rs", ".zig", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs", ".css", ".html", ".json", ".toml", ".yaml", ".yml":
		return true
	default:
		return false
	}
}

func workspaceHasEmptyProjectFiles(root string) bool {
	return len(findEmptyProjectFiles(root, 1)) > 0
}

func emptyProjectFilesRecoveryToolTask(prompt string, ledger []StructuredObjective, workingDir string) string {
	files := findEmptyProjectFiles(workingDir, 12)
	parts := []string{
		"Recovery required.",
		"Completion is blocked because empty project files remain.",
		"Required next behavior: inspect only the listed empty files if needed, then fill each with substantive source/build/test/config content or remove it if unused.",
		"Commands must target the listed paths exactly; if the listed paths are under a nested project directory, cd into that directory first or write using the full listed path.",
		"Do not use touch or mkdir.",
		"After fixing empty files, run the focused build/test/source-verification command.",
	}
	if len(files) > 0 {
		parts = append(parts, "Empty file(s): "+strings.Join(files, ",")+".")
	}
	if pending := pendingStructuredObjectiveIDs(ledger); pending != "" {
		parts = append(parts, "Pending objective(s): "+pending+".")
	}
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
}

func emptyProjectFileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() == 0
}
