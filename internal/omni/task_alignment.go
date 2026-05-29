package omni

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var graphingCalculatorDomainSignals = []string{
	"graphing calculator",
	"graph canvas",
	"plot function",
	"function plot",
	"equation input",
	"coordinate plane",
	"cartesian",
	"y =",
	"f(x)",
	"graph line",
	"axis label",
}

func promptRequestsGraphingCalculator(prompt, toolTask string) bool {
	text := strings.ToLower(strings.TrimSpace(prompt + "\n" + toolTask))
	needles := []string{
		"graphing calculator",
		"graph calculator",
		"graphing calc",
		"plot graph",
		"plot functions",
		"function graph",
		"graph functions",
		"equation graph",
		"graph equations",
		"coordinate graph",
		"cartesian graph",
	}
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return strings.Contains(text, "calculator") && (strings.Contains(text, "graph") || strings.Contains(text, "plot") || strings.Contains(text, "equation"))
}

func validateContentMatchesActivePrompt(content string, active ActivePromptContext) error {
	prompt := active.CombinedText()
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("task_alignment: generated content is empty")
	}
	item := ArchitectWorkItem{Operation: "update", CWD: ".", Path: "src/App.js"}
	contract := ImplementationArchitectContract{
		SourcePrompt:       active.UserPrompt,
		SourceToolTask:     active.ToolTask,
		AcceptanceCriteria: active.AcceptanceCriteria,
	}
	return validateArchitectContentAlignsWithPrompt(content, item, prompt, contract)
}

func validateProjectArtifactsAlignWithPrompt(workingDir string, active ActivePromptContext) error {
	if strings.TrimSpace(active.UserPrompt) == "" {
		return nil
	}
	root := strings.TrimSpace(workingDir)
	if root == "" {
		return nil
	}
	candidates := []string{
		filepath.Join(root, "src", "App.js"),
		filepath.Join(root, "src", "App.jsx"),
		filepath.Join(root, "src", "App.css"),
		filepath.Join(root, "scripts", "smoke-test.mjs"),
	}
	var issues []string
	for _, path := range candidates {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if err := validateContentMatchesActivePrompt(string(content), active); err != nil {
			issues = append(issues, fmt.Sprintf("%s: %s", filepath.ToSlash(strings.TrimPrefix(path, root+string(filepath.Separator))), err.Error()))
		}
	}
	if nested := firstNestedAppRootWithFiles(root); nested != "" && nested != "." {
		if err := validateProjectArtifactsAlignWithPrompt(filepath.Join(root, nested), active); err != nil {
			issues = append(issues, err.Error())
		}
	}
	if len(issues) == 0 {
		return nil
	}
	return fmt.Errorf("task_alignment: on-disk project artifacts do not match active prompt: %s", strings.Join(issues, "; "))
}

func rejectCompletionForArtifactMisalignment(step int, prompt, workingDir string, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	if result == nil {
		return false
	}
	active := NewActivePromptContext(prompt, "", explicitReactAppAcceptanceCriteria(prompt, ""))
	if err := validateProjectArtifactsAlignWithPrompt(workingDir, active); err != nil {
		emitStructuredCommandEvent(onEvent, "completion_check_rejected_for_task_misalignment", "Completion blocked because on-disk artifacts do not match the active prompt", map[string]string{
			"step":   fmt.Sprintf("%d", step),
			"reason": truncateStructuredTimelineValue(err.Error()),
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   err.Error(),
		})
		result.Answer = ""
		return true
	}
	return false
}
