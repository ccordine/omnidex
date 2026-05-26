package omni

import "strings"

type OperationMode string
type TaskMode string

const (
	OperationModeResearchOnly     OperationMode = "research_only"
	OperationModeInspectOnly      OperationMode = "inspect_only"
	OperationModeBuildOrVerify    OperationMode = "build_or_verify"
	OperationModeRepairProject    OperationMode = "repair_project"
	OperationModeImplementFeature OperationMode = "implement_feature"
	OperationModeCreateProject    OperationMode = "create_project"

	TaskModeResearchOnly     TaskMode = "research_only"
	TaskModeInspectOnly      TaskMode = "inspect_only"
	TaskModeBuildOrVerify    TaskMode = "build_or_verify"
	TaskModeRepairProject    TaskMode = "repair_project"
	TaskModeImplementFeature TaskMode = "implement_feature"
	TaskModeCreateProject    TaskMode = "create_project"
)

func normalizeTaskMode(mode TaskMode) TaskMode {
	switch mode {
	case TaskModeResearchOnly, TaskModeInspectOnly, TaskModeBuildOrVerify, TaskModeRepairProject, TaskModeImplementFeature, TaskModeCreateProject:
		return mode
	default:
		return ""
	}
}

func inferTaskMode(prompt string, survey WorksiteSurvey) TaskMode {
	if mode := normalizeTaskMode(survey.TaskMode); mode != "" {
		return mode
	}
	switch normalizeUserOperation(survey.UserOperation) {
	case userOperationCreateNewProject:
		return TaskModeCreateProject
	case userOperationModifyExisting:
		return TaskModeImplementFeature
	case userOperationFixExisting:
		return TaskModeRepairProject
	case userOperationRunTests:
		return TaskModeBuildOrVerify
	case userOperationInspectExisting:
		return TaskModeInspectOnly
	}
	lower := strings.ToLower(strings.TrimSpace(prompt))
	if lower == "" {
		return ""
	}
	if promptLooksResearchOnly(lower) {
		return TaskModeResearchOnly
	}
	if promptLooksInspectOnly(lower) {
		return TaskModeInspectOnly
	}
	if promptLooksRepair(lower) {
		return TaskModeRepairProject
	}
	if promptLooksBuildOrVerify(lower) {
		return TaskModeBuildOrVerify
	}
	if promptLooksCreateProject(lower) {
		return TaskModeCreateProject
	}
	if promptLooksImplementFeature(lower) {
		return TaskModeImplementFeature
	}
	return ""
}

func promptLooksResearchOnly(lower string) bool {
	if promptLooksMutationIntent(lower) {
		return false
	}
	for _, needle := range []string{
		"research ", "research-only", "research only", "look up ", "lookup ", "study ", "learn about ",
		"find docs", "read docs", "documentation", "what is ", "what are ", "explain ", "compare ",
		"investigate ", "survey options", "best practices",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func promptLooksInspectOnly(lower string) bool {
	if promptLooksMutationIntent(lower) {
		return false
	}
	for _, needle := range []string{"inspect ", "read ", "list ", "map ", "survey ", "check the code", "show me"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func promptLooksMutationIntent(lower string) bool {
	for _, needle := range []string{
		"build ", "create ", "implement ", "add ", "fix ", "repair ", "change ", "update ", "modify ",
		"install ", "write ", "patch ", "refactor ", "delete ", "remove ", "move ", "rename ",
		"run build", "run tests", "test ",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

func promptLooksRepair(lower string) bool {
	return strings.Contains(lower, "fix ") || strings.Contains(lower, "repair ") || strings.Contains(lower, "debug ")
}

func promptLooksBuildOrVerify(lower string) bool {
	return strings.Contains(lower, "run tests") || strings.Contains(lower, "test ") || strings.Contains(lower, "build ") || strings.Contains(lower, "verify ")
}

func promptLooksCreateProject(lower string) bool {
	return strings.Contains(lower, "create ") || strings.Contains(lower, "scaffold ") || strings.Contains(lower, "new project")
}

func promptLooksImplementFeature(lower string) bool {
	for _, needle := range []string{"implement ", "add ", "change ", "update ", "modify ", "refactor "} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}
