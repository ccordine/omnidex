package omni

import (
	"strconv"
	"strings"
)

func shouldForceStructuredLoopRecovery(ledger []StructuredObjective, observations []StructuredCommandObservation) bool {
	decision := ProgressionGate{}.ReviewStep(ProgressionInput{ObjectiveLedger: ledger, Observations: observations})
	return decision.Action == ProgressForceRecovery
}

func structuredLoopRecoveryToolTask(prompt string, ledger []StructuredObjective, observations []StructuredCommandObservation) string {
	if appBuildPromptNeedsFiles(prompt) || pendingObjectivesNeedSubstantiveAppFiles(ledger) {
		return writeAfterInspectionRecoveryToolTask(prompt, ledger, observations, "")
	}
	state := structuredLoopStateFromState(ledger, observations)
	pending := strings.Join(state.PendingObjectiveIDs, ",")
	if pending == "" {
		pending = pendingStructuredObjectiveIDs(ledger)
	}
	parts := []string{
		"Recovery required.",
		"A previous proposal or command did not advance the task.",
		"Choose one concrete shell command that advances the active task.",
		"Rejected proposals that did not execute are feedback only, not forbidden commands and not class bans.",
	}
	if pending != "" {
		parts = append(parts, "Active objective(s): "+pending+".")
	}
	parts = append(parts, "Required next behavior: use the observed failure reason to choose a corrected command, a different source, narrower verification, or a different command strategy appropriate to the active task.")
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
}

func pendingObjectivesNeedSubstantiveAppFiles(ledger []StructuredObjective) bool {
	for _, objective := range ledger {
		if structuredObjectiveSatisfied(objective) {
			continue
		}
		text := strings.ToLower(strings.TrimSpace(objective.ID + " " + objective.Description))
		if text == "" {
			continue
		}
		needles := []string{
			"app structure",
			"component",
			"crud",
			"entry",
			"frontend",
			"implement",
			"in-memory",
			"interface",
			"source",
			"state",
			"store",
			"storage",
			"ui",
		}
		for _, needle := range needles {
			if strings.Contains(text, needle) {
				return true
			}
		}
	}
	return false
}

func completedEvidenceRecoveryToolTask(prompt string, ledger []StructuredObjective, observations []StructuredCommandObservation, rejected string, previous StructuredCommandObservation) string {
	pending := pendingStructuredObjectiveIDs(ledger)
	parts := []string{
		"Recovery required.",
		"The proposed command already completed earlier; do not run it again.",
		"Use the previous command output as current evidence.",
		"Rejected command: " + strings.TrimSpace(rejected) + ".",
		fmtObservationForRecovery("Previous result", previous),
		"Required next behavior: choose the next unread relevant file, inspect package metadata, patch a relevant file, update the objective ledger from evidence, or choose a different concrete command.",
		"Do not return done=true while pending objectives remain.",
	}
	if pending != "" {
		parts = append(parts, "Active objective(s): "+pending+".")
		parts = append(parts, "Pending objective(s): "+pending+".")
	}
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
}

func missingFileRecoveryToolTask(prompt string, ledger []StructuredObjective, obs StructuredCommandObservation) string {
	parent := parentDirFromReadCommand(obs.Command)
	pending := pendingStructuredObjectiveIDs(ledger)
	parts := []string{
		"Recovery required.",
		"A read/inspect command failed because the target path does not exist.",
		"Invalid command: " + strings.TrimSpace(obs.Command) + ".",
		fmtObservationForRecovery("Failure", obs),
		"Required next behavior: inspect the parent directory, run a bounded file discovery command, inspect package.json if present, update the workspace model, then continue with discovered files.",
		"Do not retry the invalid path unless new evidence proves it exists.",
	}
	if parent != "" {
		parts = append(parts, "Suggested discovery: ls -la "+parent+" OR find "+parent+" -maxdepth 3 -type f.")
	}
	if pending != "" {
		parts = append(parts, "Active objective(s): "+pending+".")
		parts = append(parts, "Pending objective(s): "+pending+".")
	}
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
}

func existingScaffoldRecoveryToolTask(prompt string, ledger []StructuredObjective, obs StructuredCommandObservation, workingDir string) string {
	pending := pendingStructuredObjectiveIDs(ledger)
	targetRoot := scaffoldRootFromObservation(obs, workingDir)
	parts := []string{
		"Recovery required.",
		"The project scaffold already exists, so setup/scaffold commands must not be rerun.",
		"Failed scaffold command: " + strings.TrimSpace(obs.Command) + ".",
		fmtObservationForRecovery("Failure", obs),
		"Do not continue with generic read-only inventory commands such as ls -la.",
		"Required next behavior: create or modify the actual backend and frontend project files now.",
		"For a Go plus React app, patch existing Go server/API files, React component/source files, package scripts or Makefile targets, and automated tests/smoke checks.",
		"After source edits, run targeted verification such as go test ./..., npm test, npm run build, or make test.",
	}
	if targetRoot != "" {
		parts = append(parts, "Implementation architect target root: "+targetRoot+". All source edits, package scripts, and verification commands for this app must run inside "+targetRoot+" or use paths under "+targetRoot+"/.")
	}
	if pending != "" {
		parts = append(parts, "Active objective(s): "+pending+".")
		parts = append(parts, "Pending objective(s): "+pending+".")
	}
	if strings.TrimSpace(workingDir) != "" {
		parts = append(parts, "Current working directory: "+strings.TrimSpace(workingDir)+".")
	}
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
}

func scaffoldRootFromObservation(obs StructuredCommandObservation, workingDir string) string {
	text := obs.Command + "\n" + obs.Stdout + "\n" + obs.Stderr
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "Success! Created ") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			name := strings.Trim(parts[2], `"'`)
			if name != "" {
				return name
			}
		}
	}
	if nested := firstNestedAppRootWithFiles(workingDir); nested != "" {
		return nested
	}
	return ""
}

func dockerLifecycleRecoveryToolTask(prompt string, ledger []StructuredObjective, obs StructuredCommandObservation, workingDir string) string {
	pending := pendingStructuredObjectiveIDs(ledger)
	parts := []string{
		"Recovery required.",
		"A Dockerfile was created, but Docker lifecycle objectives are still pending.",
		fmtObservationForRecovery("Dockerfile creation", obs),
		"Do not stop after Dockerfile creation.",
		"Required next behavior: inspect the current Dockerfile and relevant package/build files, then run Docker lifecycle verification now: docker build, docker run with a named container and no restart policy, live HTTP check with curl when a port is exposed, docker inspect running/restarting/restart count, and docker logs inspection.",
		"If build or runtime fails, iterate over the Dockerfile and source/config files named in the error output, patch them, and rerun the failing Docker command.",
		"Do not return done=true until build image, run container, live app check, container state, restart count, and logs have observed success evidence.",
	}
	if pending != "" {
		parts = append(parts, "Pending objective(s): "+pending+".")
	}
	if strings.TrimSpace(workingDir) != "" {
		parts = append(parts, "Current working directory: "+strings.TrimSpace(workingDir)+".")
	}
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
}

func noProgressCommandRecoveryToolTask(prompt string, ledger []StructuredObjective, command, fingerprint string, count int) string {
	pending := pendingStructuredObjectiveIDs(ledger)
	parts := []string{
		"Recovery required.",
		"The same command produced the same result repeatedly without satisfying the pending objectives.",
		"Repeated command: " + strings.TrimSpace(command) + ".",
		"Repeat count: " + strconv.Itoa(count) + ".",
		"Output fingerprint: " + strings.TrimSpace(fingerprint) + ".",
		"Required next behavior: do not retry the same command. Use the existing evidence, inspect package.json or source files, patch the project files/config directly, choose a narrower command, or run verification that advances a pending objective.",
	}
	if strings.Contains(strings.ToLower(fingerprint), "could not determine executable to run") {
		parts = append(parts, "If this came from an npm/npx executable lookup, inspect package.json and node_modules/.bin, then configure or edit files directly instead of repeating the failing executable command.")
	}
	if pending != "" {
		parts = append(parts, "Pending objective(s): "+pending+".")
	}
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
}
