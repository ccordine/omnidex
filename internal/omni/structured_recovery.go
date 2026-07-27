package omni

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func runProgressionGateRecovery(ctx context.Context, step int, prompt string, decision ProgressionDecision, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc, result *CommandDecisionResult) (bool, error) {
	eventType := "progression_gate_forced_recovery"
	summary := "Progression gate forced alternate execution path"
	if decision.Action == ProgressUseCompletedEvidence {
		eventType = "progression_gate_use_completed_evidence"
		summary = "Progression gate reused completed command evidence and forced next action"
	}
	emitStructuredCommandEvent(onEvent, eventType, summary, map[string]string{
		"step":             fmt.Sprintf("%d", step),
		"reason":           decision.Reason,
		"rejected_command": truncateStructuredTimelineValue(decision.RejectedCommand),
	})
	if recoveryTask, ok := runThinkingForRecovery(ctx, step, prompt, decision, cfg, worksiteSurvey, result, onEvent); ok {
		decision.RecoveryToolTask = recoveryTask
		emitStructuredCommandEvent(onEvent, "thinking_recovery_adopted", "Execution layer adopting thinking layer recovery plan", map[string]string{
			"step":               fmt.Sprintf("%d", step),
			"recovery_tool_task": truncateStructuredTimelineValue(recoveryTask),
		})
	}
	gate := ProgressionGate{MaxRecoveryAttempts: 4}
	result.Observations = append(result.Observations, gate.RecoveryObservation(step, decision))
	if handled, err := runEmptyFileRecovery(ctx, step, prompt, decision, cfg, worksiteSurvey, onEvent, result); handled || err != nil {
		return handled, err
	}
	if cfg.ShellSpecialist != nil {
		if shouldBypassShellSpecialistForWriteRecovery(decision.RecoveryToolTask, result.Observations) {
			emitStructuredCommandEvent(onEvent, "progression_gate_shell_bypassed", "Shell specialist bypassed after repeated invalid write-recovery proposals", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": "planner must choose a substantive write/patch/build/test command from observed evidence",
			})
			return false, nil
		}
		if handled, err := runDelegatedShellSpecialist(ctx, step, prompt, decision.RecoveryToolTask, cfg, worksiteSurvey, stdout, stderr, onEvent, onAsk, result); handled || err != nil {
			return handled, err
		}
	}
	if handled, err := runPathfinderForProgression(ctx, step, prompt, decision, cfg, worksiteSurvey, stdout, stderr, onEvent, result); handled || err != nil {
		return handled, err
	}
	return false, nil
}

func runEmptyFileRecovery(ctx context.Context, step int, prompt string, decision ProgressionDecision, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	if !strings.Contains(strings.ToLower(decision.Reason+" "+decision.RecoveryToolTask), "empty project files") {
		return false, nil
	}
	files := findEmptyProjectFiles(cfg.CurrentWorkingDirectory, 12)
	if len(files) == 0 {
		return false, nil
	}
	emitStructuredCommandEvent(onEvent, "empty_file_recovery_started", "Empty-file recovery selected code-owned targets", map[string]string{
		"step":  fmt.Sprintf("%d", step),
		"files": strings.Join(files, ","),
	})
	if cfg.CodeContentSpecialist == nil {
		return failEmptyFileRecovery(step, files, fmt.Errorf("code content specialist is required to recover empty project files: %s", strings.Join(files, ",")), onEvent, result)
	}
	proposals := make(map[string]CodeContentProposal, len(files))
	for _, file := range files {
		item := ArchitectWorkItem{
			ID:          "fill_empty_project_file_" + sanitizeArchitectWorkItemID(file),
			Operation:   "update",
			CWD:         ".",
			Path:        file,
			Description: "Fill the exact empty project file with substantive content or remove it if unused",
		}
		proposal, err := generateValidatedCodeContent(ctx, step, prompt, ImplementationArchitectContract{
			Role:           "empty_file_recovery",
			TargetRoot:     ".",
			Framework:      primaryFrameworkFromSurvey(worksiteSurvey),
			PackageManager: worksiteSurvey.PackageManager,
			CurrentItem:    &item,
			WorkQueue:      []ArchitectWorkItem{item},
			Guardrails: []string{
				"Target path is code-owned and immutable for this operation.",
				"Generate only full file content for the provided work_item.path.",
				"Do not choose, mention, or redirect to a different file path.",
			},
		}, item, "", cfg, worksiteSurvey, onEvent, result)
		if err != nil {
			return failEmptyFileRecovery(step, files, fmt.Errorf("empty-file recovery failed for %s: %w", file, err), onEvent, result)
		}
		if strings.TrimSpace(proposal.Content) == "" {
			return failEmptyFileRecovery(step, files, fmt.Errorf("empty-file recovery specialist returned empty content for %s", file), onEvent, result)
		}
		proposals[file] = proposal
	}
	for _, file := range files {
		proposal := proposals[file]
		targetPath := filepath.Join(cfg.CurrentWorkingDirectory, filepath.FromSlash(file))
		existing, err := os.ReadFile(targetPath)
		if err != nil {
			return failEmptyFileRecovery(step, files, fmt.Errorf("read empty-file recovery target %s: %w", file, err), onEvent, result)
		}
		if len(existing) != 0 {
			return failEmptyFileRecovery(step, files, fmt.Errorf("empty-file recovery target %s changed before write; refusing to overwrite non-empty content", file), onEvent, result)
		}
		if err := os.WriteFile(targetPath, []byte(proposal.Content), 0o644); err != nil {
			return failEmptyFileRecovery(step, files, fmt.Errorf("write empty-file recovery target %s: %w", file, err), onEvent, result)
		}
		command := "empty_file.apply update " + filepath.ToSlash(file)
		emitStructuredCommandEvent(onEvent, "empty_file_recovery_applied", "Empty-file recovery wrote validated specialist content to code-owned target", map[string]string{
			"step":      fmt.Sprintf("%d", step),
			"path":      filepath.ToSlash(file),
			"rationale": truncateStructuredTimelineValue(proposal.Rationale),
		})
		result.Command = command
		result.ExitCode = 0
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			Command:  command,
			ExitCode: 0,
			Stdout:   "empty_file_recovery_applied path=" + filepath.ToSlash(file),
		})
	}
	remaining := findEmptyProjectFiles(cfg.CurrentWorkingDirectory, 12)
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		Command:  "empty_file.verify",
		ExitCode: boolToExitCode(len(remaining) == 0),
		Stdout:   fmt.Sprintf("remaining_empty_files=%d %s", len(remaining), strings.Join(remaining, ",")),
	})
	if len(remaining) > 0 {
		return failEmptyFileRecovery(step, remaining, fmt.Errorf("empty project files remain after recovery: %s", strings.Join(remaining, ",")), onEvent, result)
	}
	return true, nil
}

func failEmptyFileRecovery(step int, files []string, err error, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	emitStructuredCommandEvent(onEvent, "empty_file_recovery_failed", "Empty-file recovery failed without modifying the queued targets", map[string]string{
		"step":   fmt.Sprintf("%d", step),
		"files":  strings.Join(files, ","),
		"reason": truncateStructuredTimelineValue(err.Error()),
	})
	result.Command = "empty_file.recovery"
	result.ExitCode = 1
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		Command:  "empty_file.recovery",
		ExitCode: 1,
		Stderr:   err.Error(),
	})
	return true, err
}

func primaryFrameworkFromSurvey(survey WorksiteSurvey) string {
	for _, framework := range survey.Frameworks {
		if strings.TrimSpace(framework) != "" {
			return framework
		}
	}
	return ""
}

func boolToExitCode(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

func shouldBypassShellSpecialistForWriteRecovery(toolTask string, observations []StructuredCommandObservation) bool {
	if !toolTaskRequiresMutation(toolTask) {
		return false
	}
	rejected := 0
	for _, obs := range observations {
		text := strings.ToLower(obs.Stderr)
		if strings.Contains(text, "placeholder-only") || strings.Contains(text, "read-only command") || strings.Contains(text, "documentation download") {
			rejected++
		}
	}
	return rejected >= 2
}

func runDelegatedShellSpecialist(ctx context.Context, step int, prompt, toolTask string, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc, result *CommandDecisionResult) (bool, error) {
	if handled, err := runArchitectCodeContentLane(ctx, step, prompt, toolTask, cfg, worksiteSurvey, stdout, stderr, onEvent, result); handled || err != nil {
		return handled, err
	}
	if handled := blockShellFallbackForArchitectFileWork(step, prompt, toolTask, cfg, worksiteSurvey, onEvent, result); handled {
		return true, nil
	}
	if cfg.ShellSpecialist == nil {
		return false, nil
	}
	emitStructuredCommandEvent(onEvent, "structured_tool_delegation_started", "Planner delegated shell command selection", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"tool_task": truncateStructuredTimelineValue(toolTask),
	})
	if len(allPrepBriefs(cfg.PrepContext)) > 0 || len(cfg.PrepContext.Evidence) > 0 {
		emitStructuredCommandEvent(onEvent, "prep_context_attached_to_specialist", "Preparation context attached to shell specialist", map[string]string{
			"step":        fmt.Sprintf("%d", step),
			"role":        "shell_specialist",
			"briefs":      fmt.Sprintf("%d", len(allPrepBriefs(cfg.PrepContext))),
			"evidence":    fmt.Sprintf("%d", len(cfg.PrepContext.Evidence)),
			"route_files": strings.Join(cfg.PrepContext.CodebaseRoute.LikelyFiles, ","),
		})
	}
	proposal, ok, err := proposeValidatedShellCommand(ctx, step, prompt, toolTask, cfg, worksiteSurvey, &result.ObjectiveLedger, onEvent, onAsk, result)
	if err != nil || !ok {
		return true, err
	}
	if err := runDelegatedShellProposalWithLocalRepair(ctx, step, prompt, toolTask, proposal, cfg, worksiteSurvey, &result.ObjectiveLedger, stdout, stderr, onEvent, onAsk, result); err != nil {
		return true, err
	}
	return true, nil
}

func runDelegatedShellProposalWithLocalRepair(ctx context.Context, step int, prompt, toolTask string, proposal ShellCommandProposal, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, ledger *[]StructuredObjective, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc, result *CommandDecisionResult) error {
	if err := runStructuredPayloadCommand(ctx, step, proposal.Command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, result); err != nil {
		return err
	}
	if !latestDelegatedShellCommandFailed(result) {
		return nil
	}
	emitStructuredCommandEvent(onEvent, "structured_tool_delegation_repair_started", "Shell specialist received direct execution failure for local repair", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"command": truncateStructuredTimelineValue(proposal.Command),
		"reason":  truncateStructuredTimelineValue(latestShellRepairFeedback(result.Observations)),
	})
	repaired, ok, err := proposeValidatedShellCommand(ctx, step, prompt, toolTask, cfg, worksiteSurvey, ledger, onEvent, onAsk, result)
	if err != nil || !ok {
		return err
	}
	return runStructuredPayloadCommand(ctx, step, repaired.Command, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, result)
}

func latestDelegatedShellCommandFailed(result *CommandDecisionResult) bool {
	if result == nil || len(result.Observations) == 0 {
		return false
	}
	latest := result.Observations[len(result.Observations)-1]
	return strings.TrimSpace(latest.Command) != "" && latest.ExitCode != 0
}

func mapDrivenArchitectToolTask(workingDir string, survey WorksiteSurvey) string {
	root := architectTargetRootForWorkQueue(workingDir)
	return "Implementation architect target root: " + root + ". Create or modify the actual project files listed in project_file_map.active_file."
}
