package omni

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
