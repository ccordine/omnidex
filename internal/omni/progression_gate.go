package omni

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ProgressionAction string

const (
	ProgressAllow                ProgressionAction = "allow"
	ProgressForceRecovery        ProgressionAction = "force_recovery"
	ProgressUseCompletedEvidence ProgressionAction = "use_completed_evidence"
	ProgressRejectFalseDone      ProgressionAction = "reject_false_done"
	ProgressSwitchToPatch        ProgressionAction = "switch_to_patch"
	ProgressNarrowVerification   ProgressionAction = "narrow_verification"
	ProgressFailWithEvidence     ProgressionAction = "fail_with_evidence"
)

type ProgressionGate struct {
	MaxRecoveryAttempts int
}

type ProgressionInput struct {
	Prompt          string
	WorkingDir      string
	WorksiteSurvey  WorksiteSurvey
	ObjectiveLedger []StructuredObjective
	Observations    []StructuredCommandObservation
}

type ProgressionDecision struct {
	Action            ProgressionAction
	Reason            string
	LoopState         StructuredLoopState
	ForbiddenCommands []string
	RecoveryToolTask  string
	RejectedCommand   string
	PreviousResult    *StructuredCommandObservation
}

func (g ProgressionGate) ReviewStep(input ProgressionInput) ProgressionDecision {
	if g.MaxRecoveryAttempts <= 0 {
		g.MaxRecoveryAttempts = 4
	}
	state := structuredLoopStateFromState(input.ObjectiveLedger, input.Observations)
	decision := ProgressionDecision{
		Action:            ProgressAllow,
		LoopState:         state,
		ForbiddenCommands: state.ForbiddenCommands,
	}
	if len(pendingStructuredObjectives(input.ObjectiveLedger)) > 0 {
		if command, previous, ok := latestRepeatedSuccessEvidence(input.Observations); ok {
			decision.Action = ProgressUseCompletedEvidence
			decision.Reason = "command already completed earlier; use prior output as evidence and choose the next unread target"
			decision.RejectedCommand = command
			decision.PreviousResult = &previous
			decision.RecoveryToolTask = completedEvidenceRecoveryToolTask(input.Prompt, input.ObjectiveLedger, input.Observations, command, previous)
			return decision
		}
	}
	if objectiveLedgerHasActiveEmptyFileCleanup(input.ObjectiveLedger) && workspaceHasEmptyProjectFiles(input.WorkingDir) {
		decision.Action = ProgressForceRecovery
		decision.Reason = "empty project files remain; deterministic empty-file recovery required"
		decision.RecoveryToolTask = emptyProjectFilesRecoveryToolTask(input.Prompt, input.ObjectiveLedger, input.WorkingDir)
		return decision
	}
	if latestENOENTObservation(input.Observations) != nil {
		latest := latestENOENTObservation(input.Observations)
		decision.Action = ProgressForceRecovery
		decision.Reason = "file path was invalid; deterministic missing-file recovery required"
		decision.RejectedCommand = latest.Command
		decision.RecoveryToolTask = missingFileRecoveryToolTask(input.Prompt, input.ObjectiveLedger, *latest)
		return decision
	}
	if latest := latestExistingScaffoldObservation(input.Observations); latest != nil && appBuildPromptNeedsFiles(input.Prompt) {
		decision.Action = ProgressForceRecovery
		decision.Reason = "project scaffold already exists; continue with implementation instead of rerunning scaffold"
		decision.RejectedCommand = latest.Command
		decision.RecoveryToolTask = existingScaffoldRecoveryToolTask(input.Prompt, input.ObjectiveLedger, *latest, input.WorkingDir)
		return decision
	}
	if latest := latestDockerfileOnlyObservation(input.Observations); latest != nil && pendingDockerObjectivesNeedLifecycle(input.ObjectiveLedger) {
		decision.Action = ProgressForceRecovery
		decision.Reason = "Dockerfile exists but Docker lifecycle objectives remain unverified"
		decision.RejectedCommand = latest.Command
		decision.RecoveryToolTask = dockerLifecycleRecoveryToolTask(input.Prompt, input.ObjectiveLedger, *latest, input.WorkingDir)
		return decision
	}
	if shouldForceWriteAfterInspection(input) {
		decision.Action = ProgressForceRecovery
		decision.Reason = "workspace inspection has not produced app files; creation step is now required"
		decision.RecoveryToolTask = writeAfterInspectionRecoveryToolTask(input.Prompt, input.ObjectiveLedger, input.Observations, input.WorkingDir)
		return decision
	}
	if latest := latestPlaceholderOnlySuccess(input.Observations); latest != nil && placeholderOnlySuccessNeedsRecovery(input) {
		decision.Action = ProgressForceRecovery
		decision.Reason = "placeholder-only scaffold succeeded but substantive app files are still required"
		decision.RejectedCommand = latest.Command
		decision.RecoveryToolTask = writeAfterInspectionRecoveryToolTask(input.Prompt, input.ObjectiveLedger, input.Observations, input.WorkingDir)
		return decision
	}
	if latest := latestNonAppMutationSuccess(input.Observations); latest != nil && appBuildPromptNeedsFiles(input.Prompt) && workspaceMissingAppFiles(input.WorkingDir) {
		decision.Action = ProgressForceRecovery
		decision.Reason = "latest mutation did not create substantive app source/build/test files"
		decision.RejectedCommand = latest.Command
		decision.RecoveryToolTask = writeAfterInspectionRecoveryToolTask(input.Prompt, input.ObjectiveLedger, input.Observations, input.WorkingDir)
		return decision
	}
	if repeatedPlannerNoopForMissingAppFiles(input) {
		decision.Action = ProgressForceRecovery
		decision.Reason = "planner repeatedly failed to produce source-writing action for empty app workspace"
		decision.RecoveryToolTask = writeAfterInspectionRecoveryToolTask(input.Prompt, input.ObjectiveLedger, input.Observations, input.WorkingDir)
		return decision
	}
	if len(pendingStructuredObjectives(input.ObjectiveLedger)) > 0 {
		if command, fingerprint, count, ok := repeatedNoProgressCommand(input.Observations); ok {
			decision.Action = ProgressForceRecovery
			decision.Reason = "same command/output repeated without satisfying pending objectives; no-progress recovery required"
			decision.RejectedCommand = command
			decision.RecoveryToolTask = noProgressCommandRecoveryToolTask(input.Prompt, input.ObjectiveLedger, command, fingerprint, count)
			return decision
		}
	}
	if latestRealObservationSucceeded(input.Observations) {
		return decision
	}
	if state.Status != "blocked" || state.RepeatKind != "rejected_command" {
		return decision
	}
	if forcedRecoveryAttemptCount(input.Observations) >= g.MaxRecoveryAttempts {
		decision.Action = ProgressFailWithEvidence
		decision.Reason = "progression recovery exhausted after repeated blocked strategy"
		return decision
	}
	decision.Action = ProgressForceRecovery
	decision.Reason = "repeated command failed to advance; deterministic recovery required"
	decision.RecoveryToolTask = structuredLoopRecoveryToolTask(input.Prompt, input.ObjectiveLedger, input.Observations)
	return decision
}

func (g ProgressionGate) RecoveryObservation(step int, decision ProgressionDecision) StructuredCommandObservation {
	return StructuredCommandObservation{
		Step:            step,
		RejectedCommand: truncateStructuredObservation(decision.RejectedCommand),
		ExitCode:        1,
		Stderr:          "progression_gate: forced recovery required; " + decision.Reason,
	}
}

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

func shouldForceWriteAfterInspection(input ProgressionInput) bool {
	if !appBuildObjectiveNeedsFileCreation(input.ObjectiveLedger) {
		return false
	}
	if !appBuildPromptNeedsFiles(input.Prompt) || !workspaceMissingAppFiles(input.WorkingDir) {
		return false
	}
	return len(successfulReadOnlyStructuredCommands(input.Observations)) >= 2 && !hasSuccessfulStructuredMutation(input.Observations)
}

func pendingDockerObjectivesNeedLifecycle(ledger []StructuredObjective) bool {
	for _, objective := range pendingStructuredObjectives(ledger) {
		text := strings.ToLower(objective.ID + " " + objective.Description)
		if (strings.Contains(text, "docker") || strings.Contains(text, "container")) &&
			(strings.Contains(text, "build") || strings.Contains(text, "run") || strings.Contains(text, "compatibility") || strings.Contains(text, "dependencies") || strings.Contains(text, "image")) {
			return true
		}
	}
	return false
}

func latestDockerfileOnlyObservation(observations []StructuredCommandObservation) *StructuredCommandObservation {
	if len(observations) == 0 {
		return nil
	}
	latest := observations[len(observations)-1]
	if latest.ExitCode != 0 || strings.TrimSpace(latest.Command) == "" {
		return nil
	}
	text := strings.ToLower(latest.Command + "\n" + latest.Stdout + "\n" + latest.Stderr)
	if !strings.Contains(text, "dockerfile") {
		return nil
	}
	if strings.Contains(text, "docker build") || strings.Contains(text, "docker run") || strings.Contains(text, "docker inspect") || strings.Contains(text, "docker logs") {
		return nil
	}
	return &latest
}

func appBuildObjectiveNeedsFileCreation(ledger []StructuredObjective) bool {
	if len(ledger) == 0 {
		return true
	}
	for _, objective := range pendingStructuredObjectives(ledger) {
		text := strings.ToLower(objective.ID + " " + objective.Description)
		if strings.Contains(text, "cleanup") || strings.Contains(text, "clean up") || strings.Contains(text, "remove_empty") || strings.Contains(text, "placeholder") {
			continue
		}
		if strings.Contains(text, "create") || strings.Contains(text, "implement") || strings.Contains(text, "build app") || strings.Contains(text, "project files") || strings.Contains(text, "ui") {
			return true
		}
	}
	return false
}

func appBuildPromptNeedsFiles(prompt string) bool {
	prompt = strings.ToLower(strings.TrimSpace(prompt))
	if strings.Contains(prompt, "calculator app") {
		return true
	}
	buildVerbs := []string{"build", "create", "implement", "scaffold", "make", "develop", "write"}
	fileTargets := []string{"app", "project", "javascript", "react", "html", "ui", "website", "cli", "frontend", "component"}
	hasBuildVerb := false
	hasFileTarget := false
	for _, needle := range buildVerbs {
		if strings.Contains(prompt, needle) {
			hasBuildVerb = true
			break
		}
	}
	for _, needle := range fileTargets {
		if strings.Contains(prompt, needle) {
			hasFileTarget = true
			break
		}
	}
	return hasBuildVerb && hasFileTarget
}

func placeholderOnlySuccessNeedsRecovery(input ProgressionInput) bool {
	if appBuildPromptNeedsFiles(input.Prompt) {
		return true
	}
	if objectiveLedgerNeedsSubstantiveAppFiles(input.ObjectiveLedger) {
		return true
	}
	if shouldScanEmptyProjectFiles(input.Prompt, input.ObjectiveLedger, input.Observations) {
		return true
	}
	if workspaceHasEmptyProjectFiles(input.WorkingDir) {
		return true
	}
	return workspaceMissingAppFiles(input.WorkingDir)
}

func workspaceMissingAppFiles(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	if nested := firstNestedAppRootWithFiles(root); nested != "" {
		return false
	}
	hasWebEntrypoint := fileHasContent(filepath.Join(root, "src", "index.js")) ||
		fileHasContent(filepath.Join(root, "src", "main.js")) ||
		fileHasContent(filepath.Join(root, "src", "index.jsx")) ||
		fileHasContent(filepath.Join(root, "src", "main.jsx"))
	hasZigEntrypoint := fileHasContent(filepath.Join(root, "src", "main.zig"))
	hasZigBuild := fileHasContent(filepath.Join(root, "build.zig")) || fileHasContent(filepath.Join(root, "build.zig.zon"))
	if hasZigEntrypoint && hasZigBuild {
		return false
	}
	hasRustManifest := fileHasContent(filepath.Join(root, "Cargo.toml"))
	hasRustEntrypoint := fileHasContent(filepath.Join(root, "src", "main.rs")) || fileHasContent(filepath.Join(root, "src", "lib.rs"))
	if hasRustManifest && hasRustEntrypoint {
		return false
	}
	if fileHasContent(filepath.Join(root, "index.html")) && hasWebEntrypoint {
		return false
	}
	return true
}

func firstNestedAppRootWithFiles(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() || shouldSkipEmptyFileScanDir(entry.Name()) {
			continue
		}
		candidate := filepath.Join(root, entry.Name())
		if !fileHasContent(filepath.Join(candidate, "package.json")) &&
			!fileHasContent(filepath.Join(candidate, "Cargo.toml")) &&
			!fileHasContent(filepath.Join(candidate, "go.mod")) &&
			!fileHasContent(filepath.Join(candidate, "build.zig")) {
			continue
		}
		if !workspaceMissingAppFilesShallow(candidate) {
			return entry.Name()
		}
	}
	return ""
}

func workspaceMissingAppFilesShallow(root string) bool {
	hasWebEntrypoint := fileHasContent(filepath.Join(root, "src", "index.js")) ||
		fileHasContent(filepath.Join(root, "src", "main.js")) ||
		fileHasContent(filepath.Join(root, "src", "index.jsx")) ||
		fileHasContent(filepath.Join(root, "src", "main.jsx"))
	if fileHasContent(filepath.Join(root, "index.html")) && hasWebEntrypoint {
		return false
	}
	if fileHasContent(filepath.Join(root, "public", "index.html")) && hasWebEntrypoint && fileHasContent(filepath.Join(root, "package.json")) {
		return false
	}
	if (fileHasContent(filepath.Join(root, "Cargo.toml")) && (fileHasContent(filepath.Join(root, "src", "main.rs")) || fileHasContent(filepath.Join(root, "src", "lib.rs")))) ||
		(fileHasContent(filepath.Join(root, "go.mod")) && hasAnyFileWithExt(filepath.Join(root), ".go")) ||
		((fileHasContent(filepath.Join(root, "build.zig")) || fileHasContent(filepath.Join(root, "build.zig.zon"))) && fileHasContent(filepath.Join(root, "src", "main.zig"))) {
		return false
	}
	return true
}

func hasAnyFileWithExt(root, ext string) bool {
	found := false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if entry.IsDir() {
			if shouldSkipEmptyFileScanDir(entry.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ext) && fileHasContent(path) {
			found = true
		}
		return nil
	})
	return found
}

func fileHasContent(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() > 0
}

func successfulReadOnlyStructuredCommands(observations []StructuredCommandObservation) []string {
	commands := []string{}
	seen := map[string]bool{}
	for _, obs := range observations {
		command := strings.TrimSpace(obs.Command)
		if obs.ExitCode != 0 || command == "" || structuredCommandLooksMutating(command) {
			continue
		}
		key := normalizeStructuredCommandForComparison(command)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		commands = append(commands, command)
	}
	return commands
}

func hasSuccessfulStructuredMutation(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if obs.ExitCode == 0 && structuredCommandLooksAppFileMutation(obs.Command) {
			return true
		}
	}
	return false
}

func latestNonAppMutationSuccess(observations []StructuredCommandObservation) *StructuredCommandObservation {
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode != 0 || strings.TrimSpace(obs.Command) == "" {
			continue
		}
		if structuredCommandLooksMutating(obs.Command) && !structuredCommandLooksAppFileMutation(obs.Command) {
			return &obs
		}
		return nil
	}
	return nil
}

func repeatedPlannerNoopForMissingAppFiles(input ProgressionInput) bool {
	if !appBuildPromptNeedsFiles(input.Prompt) || !workspaceMissingAppFiles(input.WorkingDir) {
		return false
	}
	count := 0
	for _, obs := range input.Observations {
		text := strings.ToLower(obs.RejectedResponse + "\n" + obs.EvaluationFeedback + "\n" + obs.Stderr)
		if obs.RejectedResponse != "" && !structuredCommandLooksAppFileMutation(obs.RejectedCommand) && (strings.Contains(text, "empty") || strings.Contains(text, "no meaningful project files") || strings.Contains(text, "initialize") || strings.Contains(text, "项目文件") || strings.Contains(text, "初始化") || strings.Contains(text, "zig")) {
			count++
		}
	}
	return count >= 2
}

func structuredCommandLooksAppFileMutation(command string) bool {
	if shellProposalIsPlaceholderOnlyMutation(command) {
		return false
	}
	lower := strings.ToLower(command)
	appNeedles := []string{
		"src/", "build.zig", "build.zig.zon", "package.json", "index.html", "makefile", "go.mod", "cargo.toml",
		"test", "spec", "zig build", "go test", "cargo test", "npm run build", "npm test",
	}
	for _, needle := range appNeedles {
		if strings.Contains(lower, needle) {
			return structuredCommandLooksMutating(command)
		}
	}
	return false
}

func latestPlaceholderOnlySuccess(observations []StructuredCommandObservation) *StructuredCommandObservation {
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode == 0 && strings.TrimSpace(obs.Command) != "" && shellProposalIsPlaceholderOnlyMutation(obs.Command) {
			return &obs
		}
	}
	return nil
}

func structuredCommandLooksMutating(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" {
		return false
	}
	lower := strings.ToLower(command)
	mutationNeedles := []string{
		">", "tee ", "cat <<", "node <<", "python <<", "python3 <<", "apply_patch",
		"architect.apply", "empty_file.apply",
		"npm install", "npm pkg set", "npm init", "mkdir", "touch", "cp ", "mv ", "rm ",
		" -delete", "webpack", "npm run build", "npm test",
		"writefile", "writefilesync", "appendfile", "appendfilesync", "renamesync", "copyfilesync",
		"unlinksync", "rmsync", "mkdirsync",
		"write_text", "write_bytes",
	}
	for _, needle := range mutationNeedles {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	fields := strings.Fields(lower)
	if len(fields) == 0 {
		return false
	}
	if cleanCommandPathToken(fields[0]) == "sed" {
		for _, field := range fields[1:] {
			if strings.HasPrefix(field, "-i") {
				return true
			}
		}
	}
	return false
}

func latestRepeatedSuccessEvidence(observations []StructuredCommandObservation) (string, StructuredCommandObservation, bool) {
	if len(observations) == 0 {
		return "", StructuredCommandObservation{}, false
	}
	latest := observations[len(observations)-1]
	if !strings.HasPrefix(strings.TrimSpace(latest.Command), "SKIPPED_REPEAT_SUCCESS:") || strings.TrimSpace(latest.RejectedCommand) == "" {
		return "", StructuredCommandObservation{}, false
	}
	previous, ok := previousSuccessfulStructuredCommandObservation(latest.RejectedCommand, observations[:len(observations)-1])
	return latest.RejectedCommand, previous, ok
}

func latestRealObservationSucceeded(observations []StructuredCommandObservation) bool {
	if len(observations) == 0 {
		return false
	}
	latest := observations[len(observations)-1]
	command := strings.TrimSpace(latest.Command)
	return latest.ExitCode == 0 && command != "" && !strings.HasPrefix(command, "SKIPPED_REPEAT_SUCCESS:")
}

func repeatedNoProgressCommand(observations []StructuredCommandObservation) (string, string, int, bool) {
	if len(observations) == 0 {
		return "", "", 0, false
	}
	latest := observations[len(observations)-1]
	latestCommand := normalizeStructuredCommandForComparison(latest.Command)
	if latestCommand == "" || strings.HasPrefix(strings.TrimSpace(latest.Command), "SKIPPED_REPEAT_SUCCESS:") {
		return "", "", 0, false
	}
	fingerprint := structuredFailureFingerprint(latest)
	count := 0
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if strings.TrimSpace(obs.Command) == "" || strings.HasPrefix(strings.TrimSpace(obs.Command), "SKIPPED_REPEAT_SUCCESS:") {
			continue
		}
		if normalizeStructuredCommandForComparison(obs.Command) != latestCommand {
			continue
		}
		if structuredFailureFingerprint(obs) != fingerprint {
			continue
		}
		count++
	}
	if latest.ExitCode != 0 && count >= 2 {
		return strings.TrimSpace(latest.Command), fingerprint, count, true
	}
	if latest.ExitCode == 0 && count >= 3 && commandLooksLikeNoProgressPackageManager(latest.Command, latest.Stdout, latest.Stderr) {
		return strings.TrimSpace(latest.Command), fingerprint, count, true
	}
	return "", "", count, false
}

func commandLooksLikeNoProgressPackageManager(command, stdout, stderr string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	switch cleanCommandPathToken(fields[0]) {
	case "npm", "pnpm", "yarn":
	default:
		return false
	}
	text := strings.ToLower(stdout + "\n" + stderr)
	if strings.Contains(text, "up to date") || strings.Contains(text, "already up to date") || strings.Contains(text, "audited ") {
		return true
	}
	return false
}

func latestENOENTObservation(observations []StructuredCommandObservation) *StructuredCommandObservation {
	if len(observations) == 0 {
		return nil
	}
	latest := observations[len(observations)-1]
	if latest.ExitCode == 0 || strings.TrimSpace(latest.Command) == "" {
		return nil
	}
	text := strings.ToLower(latest.Stderr + "\n" + latest.Stdout)
	if !strings.Contains(text, "no such file or directory") && !strings.Contains(text, "cannot access") && !strings.Contains(text, "no such file") {
		return nil
	}
	if !looksLikeReadCommand(latest.Command) {
		return nil
	}
	return &latest
}

func latestExistingScaffoldObservation(observations []StructuredCommandObservation) *StructuredCommandObservation {
	if len(observations) == 0 {
		return nil
	}
	latest := observations[len(observations)-1]
	if latest.ExitCode == 0 || strings.TrimSpace(latest.Command) == "" {
		return nil
	}
	commandLower := strings.ToLower(latest.Command)
	if !strings.Contains(commandLower, "go mod init") && !strings.Contains(commandLower, "create-react-app") && !strings.Contains(commandLower, "npm create") {
		return nil
	}
	text := strings.ToLower(latest.Stderr + "\n" + latest.Stdout)
	alreadyExistsNeedles := []string{
		"go.mod already exists",
		"already exists",
		"contains files that could conflict",
		"the directory",
	}
	for _, needle := range alreadyExistsNeedles {
		if strings.Contains(text, needle) {
			return &latest
		}
	}
	return nil
}

func looksLikeReadCommand(command string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	switch cleanCommandPathToken(fields[0]) {
	case "cat", "sed", "head", "tail", "stat", "ls", "test":
		return true
	default:
		return false
	}
}

func parentDirFromReadCommand(command string) string {
	fields := strings.Fields(strings.TrimSpace(command))
	for i := len(fields) - 1; i >= 0; i-- {
		token := strings.Trim(fields[i], `"'`)
		if strings.HasPrefix(token, "-") || strings.Contains(token, "=") {
			continue
		}
		if strings.Contains(token, "/") {
			if idx := strings.LastIndex(token, "/"); idx > 0 {
				return token[:idx]
			}
		}
	}
	return ""
}

func fmtObservationForRecovery(label string, obs StructuredCommandObservation) string {
	parts := []string{label + ":"}
	if obs.Step > 0 {
		parts = append(parts, "step="+strconv.Itoa(obs.Step))
	}
	if strings.TrimSpace(obs.Command) != "" {
		parts = append(parts, "command="+strings.TrimSpace(obs.Command))
	}
	parts = append(parts, "exit_code="+strconv.Itoa(obs.ExitCode))
	if strings.TrimSpace(obs.Stdout) != "" {
		parts = append(parts, "stdout="+truncateStructuredTimelineValue(obs.Stdout))
	}
	if strings.TrimSpace(obs.Stderr) != "" {
		parts = append(parts, "stderr="+truncateStructuredTimelineValue(obs.Stderr))
	}
	return strings.Join(parts, " ")
}

func structuredFailureFingerprint(obs StructuredCommandObservation) string {
	text := strings.TrimSpace(obs.Stderr)
	if text == "" {
		text = strings.TrimSpace(obs.Stdout)
	}
	if text == "" {
		return "exit_code=" + strconv.Itoa(obs.ExitCode)
	}
	return truncateStructuredTimelineValue(strings.Join(strings.Fields(text), " "))
}

func forcedRecoveryAttemptCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.Contains(obs.Stderr, "progression_gate: forced recovery required") {
			count++
		}
	}
	return count
}
