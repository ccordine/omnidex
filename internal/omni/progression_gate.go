package omni

import (
	"sort"
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
		g.MaxRecoveryAttempts = 2
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
			decision.ForbiddenCommands = appendForbiddenCommand(decision.ForbiddenCommands, command)
			decision.RecoveryToolTask = completedEvidenceRecoveryToolTask(input.Prompt, input.ObjectiveLedger, input.Observations, command, previous)
			return decision
		}
	}
	if latestRealObservationSucceeded(input.Observations) {
		return decision
	}
	if latestENOENTObservation(input.Observations) != nil {
		latest := latestENOENTObservation(input.Observations)
		decision.Action = ProgressForceRecovery
		decision.Reason = "file path was invalid; deterministic missing-file recovery required"
		decision.RejectedCommand = latest.Command
		decision.ForbiddenCommands = appendForbiddenCommand(decision.ForbiddenCommands, latest.Command)
		decision.RecoveryToolTask = missingFileRecoveryToolTask(input.Prompt, input.ObjectiveLedger, *latest)
		return decision
	}
	if state.Status != "blocked" || state.RepeatKind != "rejected_command" || len(state.ForbiddenCommands) == 0 {
		return decision
	}
	if forcedRecoveryAttemptCount(input.Observations) >= g.MaxRecoveryAttempts {
		decision.Action = ProgressFailWithEvidence
		decision.Reason = "progression recovery exhausted after repeated blocked strategy"
		return decision
	}
	decision.Action = ProgressForceRecovery
	decision.Reason = "repeated command exhausted; deterministic recovery required"
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
	state := structuredLoopStateFromState(ledger, observations)
	pending := strings.Join(state.PendingObjectiveIDs, ",")
	if pending == "" {
		pending = pendingStructuredObjectiveIDs(ledger)
	}
	forbidden := strings.Join(state.ForbiddenCommands, "; ")
	parts := []string{
		"Recovery required.",
		"The previous command is exhausted and must not be repeated.",
		"Choose one concrete shell command that changes strategy and advances the active task.",
	}
	if pending != "" {
		parts = append(parts, "Active objective(s): "+pending+".")
	}
	if forbidden != "" {
		parts = append(parts, "Blocked command(s): "+forbidden+".")
		parts = append(parts, "Forbidden command(s): "+forbidden+".")
	}
	parts = append(parts, "Required next behavior: inspect existing files, patch existing project files, narrow verification, or use a different command strategy.")
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
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
		parts = append(parts, "Pending objective(s): "+pending+".")
	}
	if strings.TrimSpace(prompt) != "" {
		parts = append(parts, "Active task: "+strings.TrimSpace(prompt)+".")
	}
	return strings.Join(parts, " ")
}

func exhaustedStructuredCommands(observations []StructuredCommandObservation) []string {
	rejected := map[string]struct{}{}
	failedFingerprints := map[string]int{}
	original := map[string]string{}
	for _, obs := range observations {
		if obs.ExitCode == 0 {
			continue
		}
		if normalized := normalizeStructuredCommandForComparison(obs.RejectedCommand); normalized != "" {
			rejected[normalized] = struct{}{}
			if _, ok := original[normalized]; !ok {
				original[normalized] = strings.TrimSpace(obs.RejectedCommand)
			}
		}
		normalized := normalizeStructuredCommandForComparison(obs.Command)
		if normalized == "" {
			continue
		}
		key := normalized + "\x00" + structuredFailureFingerprint(obs)
		failedFingerprints[key]++
		if _, ok := original[normalized]; !ok {
			original[normalized] = strings.TrimSpace(obs.Command)
		}
	}
	exhausted := map[string]struct{}{}
	for normalized := range rejected {
		exhausted[normalized] = struct{}{}
	}
	for key, count := range failedFingerprints {
		if count < 2 {
			continue
		}
		normalized, _, _ := strings.Cut(key, "\x00")
		exhausted[normalized] = struct{}{}
	}
	commands := []string{}
	for normalized := range exhausted {
		commands = append(commands, original[normalized])
	}
	sort.Strings(commands)
	return commands
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

func appendForbiddenCommand(commands []string, command string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return commands
	}
	for _, existing := range commands {
		if normalizeStructuredCommandForComparison(existing) == normalizeStructuredCommandForComparison(command) {
			return commands
		}
	}
	return append(commands, command)
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
