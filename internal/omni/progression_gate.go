package omni

import (
	"sort"
	"strconv"
	"strings"
)

type ProgressionAction string

const (
	ProgressAllow              ProgressionAction = "allow"
	ProgressForceRecovery      ProgressionAction = "force_recovery"
	ProgressSwitchToPatch      ProgressionAction = "switch_to_patch"
	ProgressNarrowVerification ProgressionAction = "narrow_verification"
	ProgressFailWithEvidence   ProgressionAction = "fail_with_evidence"
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
		Step:     step,
		ExitCode: 1,
		Stderr:   "progression_gate: forced recovery required; " + decision.Reason,
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
