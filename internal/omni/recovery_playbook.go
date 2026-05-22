package omni

import "strings"

func writeAfterInspectionRecoveryToolTask(prompt string, ledger []StructuredObjective, observations []StructuredCommandObservation, workingDir string) string {
	pending := pendingStructuredObjectiveIDs(ledger)
	readOnly := strings.Join(successfulReadOnlyStructuredCommands(observations), "; ")
	targetRoot := firstNestedAppRootWithFiles(workingDir)
	parts := []string{
		"Recovery required.",
		"The workspace has been inspected enough for this app-building task, but required app files are still missing or empty.",
		"Do not continue with read-only inventory commands.",
		"Use existing inspection evidence; inspect existing files only when needed to target a concrete patch.",
		"Required next behavior: create or modify the actual project files now, preferably with tool=patch.apply or one concrete here-doc command that writes a focused failing test/probe first, then substantive source, build metadata, tests, and verification files appropriate to the requested language/framework.",
		"If the language/framework shape is unfamiliar, use official documentation or installed tool help to create the smallest hello-world project first, then iterate from build/test errors into the requested app.",
		"Do not create placeholder-only files with touch or empty mkdir scaffolds.",
		"After the write step, run readback and verification commands appropriate to the project, such as compiler build/test commands or a deterministic source verifier when the requested compiler is unavailable.",
	}
	if targetRoot != "" {
		parts = append(parts, "Implementation architect target root: "+targetRoot+". All source edits, package scripts, and verification commands for this app must run inside "+targetRoot+" or use paths under "+targetRoot+"/.")
	}
	if pending != "" {
		parts = append(parts, "Active objective(s): "+pending+".")
		parts = append(parts, "Pending objective(s): "+pending+".")
	}
	if readOnly != "" {
		parts = append(parts, "Already completed read-only command(s): "+readOnly+".")
	}
	if strings.TrimSpace(workingDir) != "" {
		parts = append(parts, "Current working directory: "+strings.TrimSpace(workingDir)+".")
	}
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
}
