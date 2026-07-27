package omni

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func structuredCommandAttemptNumber(result *CommandDecisionResult, command string) int {
	attempt := 1
	if result == nil {
		return attempt
	}
	normalized := normalizeStructuredCommandForComparison(command)
	for _, obs := range result.Observations {
		if strings.TrimSpace(obs.Command) == "" {
			continue
		}
		if normalizeStructuredCommandForComparison(obs.Command) == normalized {
			attempt++
		}
	}
	return attempt
}

func structuredCommandID(step, attempt int, command, workingDirectory string) string {
	cwd := structuredPromptWorkingDirectory(workingDirectory)
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d\x00%d\x00%s\x00%s", step, attempt, cwd, strings.TrimSpace(command))))
	return fmt.Sprintf("cmd_%d_%d_%x", step, attempt, sum[:6])
}

func runStructuredPayloadCommand(ctx context.Context, step int, command, workingDirectory string, enableCommandCache bool, commandCacheRoot string, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) error {
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	command = normalizeStructuredCommand(command)
	attempt := structuredCommandAttemptNumber(result, command)
	commandID := structuredCommandID(step, attempt, command, workingDirectory)
	childJobID, objectiveID := activeCommandOwner(result)
	if result != nil && len(result.ChildJobs) > 0 && strings.TrimSpace(childJobID) == "" {
		emitStructuredCommandEvent(onEvent, "command_observation_missing_child_job", "Command observation has no active child job owner", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"command": truncateStructuredTimelineValue(command),
		})
	} else if strings.TrimSpace(childJobID) != "" {
		emitStructuredCommandEvent(onEvent, "command_bound_to_child_job", "Command bound to active child job", map[string]string{
			"step":         fmt.Sprintf("%d", step),
			"command_id":   commandID,
			"child_job_id": childJobID,
			"objective_id": objectiveID,
		})
	}
	if result != nil && result.TaskMode == TaskModeResearchOnly {
		if err := validateStructuredCommandForTaskMode(command, "", result.TaskMode); err != nil {
			result.Command = command
			result.ExitCode = 1
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:            step,
				CommandID:       commandID,
				ChildJobID:      childJobID,
				ObjectiveID:     objectiveID,
				RejectedCommand: truncateStructuredObservation(command),
				ExitCode:        1,
				Stderr:          "command rejected: " + err.Error(),
				CWD:             structuredPromptWorkingDirectory(workingDirectory),
				Attempt:         attempt,
			})
			emitStructuredCommandEvent(onEvent, "research_only_mutation_rejected", "Command rejected by research-only mode", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(command),
				"reason":  err.Error(),
			})
			return nil
		}
	}
	moveSpec, hasMoveSpec := mutationReconciliationMoveSpec(command, workingDirectory)
	if hasMoveSpec {
		handled, gateErr := applyMutationReconciliationGateBeforeMove(step, command, workingDirectory, mutationCommandContext{
			CommandID:   commandID,
			ChildJobID:  childJobID,
			ObjectiveID: objectiveID,
			Attempt:     attempt,
		}, moveSpec, onEvent, result)
		if handled || gateErr != nil {
			return gateErr
		}
	}
	if enableCommandCache {
		hit, err := appendCachedStructuredCommandObservation(step, attempt, commandID, command, workingDirectory, commandCacheRoot, stdout, stderr, onEvent, result)
		if err != nil {
			emitStructuredCommandEvent(onEvent, "command_cache_miss", "Command cache lookup failed; executing command", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": truncateStructuredTimelineValue(err.Error()),
			})
		} else if hit {
			return nil
		}
	}
	emitStructuredCommandEvent(onEvent, "structured_command_started", "Executing structured command", map[string]string{
		"step":       fmt.Sprintf("%d", step),
		"command_id": commandID,
		"attempt":    fmt.Sprintf("%d", attempt),
		"tool":       "shell",
		"command":    truncateStructuredTimelineValue(command),
		"cwd":        structuredPromptWorkingDirectory(workingDirectory),
	})
	started := nowUTC()
	exitCode, err := ExecuteStructuredCommandInDir(ctx, command, workingDirectory, io.MultiWriter(stdout, &stdoutBuf), io.MultiWriter(stderr, &stderrBuf))
	finished := nowUTC()
	if exitCode == 0 && structuredCommandHasPartialFailure(command, stdoutBuf.String(), stderrBuf.String()) {
		exitCode = 1
		if strings.TrimSpace(stderrBuf.String()) != "" {
			stderrBuf.WriteString("\n")
		}
		stderrBuf.WriteString("partial_failure: compound mutation command reported a failed sub-command")
		emitStructuredCommandEvent(onEvent, "structured_command_partial_failure_classified", "Compound command reported a failed mutation sub-command", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"command": truncateStructuredTimelineValue(command),
		})
	}
	if classifiedExit, partialReason := classifyPlaceholderOnlyMutationAsFailure(command, workingDirectory, exitCode); partialReason != "" {
		exitCode = classifiedExit
		if strings.TrimSpace(stderrBuf.String()) != "" {
			stderrBuf.WriteString("\n")
		}
		stderrBuf.WriteString(partialReason)
		emitStructuredCommandEvent(onEvent, "structured_command_placeholder_mutation_rejected", "Placeholder-only mutation did not produce substantive project evidence", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"command": truncateStructuredTimelineValue(command),
			"reason":  truncateStructuredTimelineValue(partialReason),
		})
	}
	if exitCode == 0 && hasMoveSpec {
		if !verifyMutationReconciliationGateAfterMove(step, command, workingDirectory, moveSpec, &stdoutBuf, &stderrBuf, onEvent, result) {
			exitCode = 1
		}
	}
	if exitCode == 0 {
		applyScaffoldTargetRootPromotion(step, command, workingDirectory, &stdoutBuf, onEvent, result)
	}
	result.Command = command
	result.ExitCode = exitCode
	feedback := ClassifyToolchainFeedback(command, stdoutBuf.String(), stderrBuf.String())
	observation := StructuredCommandObservation{
		Step:        step,
		CommandID:   commandID,
		ChildJobID:  childJobID,
		ObjectiveID: objectiveID,
		Command:     command,
		ExitCode:    exitCode,
		Stdout:      truncateStructuredObservation(stdoutBuf.String()),
		Stderr:      truncateStructuredObservation(stderrBuf.String()),
		CWD:         structuredPromptWorkingDirectory(workingDirectory),
		Attempt:     attempt,
		StartedAt:   started,
		FinishedAt:  finished,
	}
	if feedback.Kind != "" {
		observation.CapabilityMemory = strings.Join(append([]string{
			"toolchain_feedback",
			"toolchain=" + feedback.Toolchain,
			"kind=" + feedback.Kind,
			"summary=" + feedback.Summary,
		}, feedback.Hints...), "; ")
	}
	result.Observations = append(result.Observations, observation)
	emitStructuredCommandEvent(onEvent, "structured_command_finished", "Structured command finished", map[string]string{
		"step":        fmt.Sprintf("%d", step),
		"command_id":  commandID,
		"attempt":     fmt.Sprintf("%d", attempt),
		"tool":        "shell",
		"command":     truncateStructuredTimelineValue(command),
		"cwd":         structuredPromptWorkingDirectory(workingDirectory),
		"exit_code":   fmt.Sprintf("%d", exitCode),
		"stdout":      structuredTimelineCommandOutput(stdoutBuf.String()),
		"stderr":      structuredTimelineCommandOutput(stderrBuf.String()),
		"started_at":  started,
		"finished_at": finished,
	})
	if exitCode == 0 && structuredCommandIsDependencyInstall(command) {
		emitStructuredCommandEvent(onEvent, "dependencies_installed", "Dependency install command completed successfully", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"command_id": commandID,
			"command":    truncateStructuredTimelineValue(command),
		})
	}
	if feedback.Kind != "" {
		emitStructuredCommandEvent(onEvent, "toolchain_feedback_classified", "Typed compiler/test feedback classified", map[string]string{
			"step":      fmt.Sprintf("%d", step),
			"toolchain": feedback.Toolchain,
			"kind":      feedback.Kind,
			"summary":   feedback.Summary,
			"hints":     strings.Join(feedback.Hints, "; "),
		})
		if result != nil && exitCode != 0 && result.TaskMode == TaskModeResearchOnly {
			emitStructuredCommandEvent(onEvent, "toolchain_feedback_recorded_research_only", "Toolchain feedback recorded as incidental research finding without repair", map[string]string{
				"step":      fmt.Sprintf("%d", step),
				"toolchain": feedback.Toolchain,
				"kind":      feedback.Kind,
			})
		} else if result != nil && exitCode != 0 {
			result.ChildJobs, _ = upsertToolchainRepairChildJob(result.ChildJobs, feedback, result.Observations[len(result.Observations)-1], workingDirectory)
			job := result.ChildJobs[len(result.ChildJobs)-1]
			for _, candidate := range result.ChildJobs {
				if candidate.ID == toolchainRepairChildJobID(feedback) {
					job = candidate
					break
				}
			}
			emitToolchainRepairControlFlowEvents(step, feedback, job, onEvent)
		}
	}
	if enableCommandCache {
		if err := saveStructuredCommandCache(command, workingDirectory, commandCacheRoot, exitCode, stdoutBuf.String(), stderrBuf.String(), onEvent); err != nil {
			emitStructuredCommandEvent(onEvent, "command_cache_store_failed", "Command cache store failed", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": truncateStructuredTimelineValue(err.Error()),
			})
		}
	}
	return err
}

func structuredCommandIsDependencyInstall(command string) bool {
	lower := strings.ToLower(strings.TrimSpace(command))
	return strings.HasPrefix(lower, "npm install") ||
		strings.Contains(lower, " npm install ") ||
		strings.HasPrefix(lower, "pnpm install") ||
		strings.Contains(lower, " pnpm install ") ||
		strings.HasPrefix(lower, "yarn install") ||
		strings.HasPrefix(lower, "yarn add") ||
		strings.Contains(lower, " yarn add ")
}

func runStructuredPatchApply(ctx context.Context, step int, patch, workingDirectory string, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) error {
	childJobID, objectiveID := activeCommandOwner(result)
	if result != nil && result.TaskMode == TaskModeResearchOnly {
		err := fmt.Errorf("research_only mode forbids code patches and source writes; record project issues as incidental findings unless the user explicitly asks for repair")
		result.Command = "PATCH_APPLY_REJECTED"
		result.ExitCode = 1
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:             step,
			ChildJobID:       childJobID,
			ObjectiveID:      objectiveID,
			RejectedResponse: truncateStructuredObservation(patch),
			ExitCode:         1,
			Stderr:           "command rejected: " + err.Error(),
		})
		emitStructuredCommandEvent(onEvent, "research_only_mutation_rejected", "Patch apply rejected by research-only mode", map[string]string{
			"step":   fmt.Sprintf("%d", step),
			"reason": err.Error(),
		})
		return nil
	}
	emitStructuredCommandEvent(onEvent, "structured_patch_apply_started", "Applying structured patch artifact", map[string]string{
		"step": fmt.Sprintf("%d", step),
		"tool": "patch.apply",
	})
	applyResult, err := ApplyUnifiedPatch(PatchApplyOptions{
		Workspace: workingDirectory,
		Patch:     patch,
	})
	exitCode := 0
	var stdoutText string
	var stderrText string
	if err != nil {
		exitCode = 1
		stderrText = err.Error()
		_, _ = io.WriteString(stderr, stderrText)
	} else {
		stdoutText = FormatPatchApplyResult(applyResult)
		_, _ = io.WriteString(stdout, stdoutText)
	}
	result.Command = "PATCH_APPLY"
	result.ExitCode = exitCode
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:        step,
		ChildJobID:  childJobID,
		ObjectiveID: objectiveID,
		Command:     "PATCH_APPLY",
		ExitCode:    exitCode,
		Stdout:      truncateStructuredObservation(stdoutText),
		Stderr:      truncateStructuredObservation(stderrText),
	})
	details := map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"tool":      "patch.apply",
		"exit_code": fmt.Sprintf("%d", exitCode),
	}
	if err == nil {
		details["files"] = fmt.Sprintf("%d", len(applyResult.Files))
	}
	if err != nil {
		details["stderr"] = truncateStructuredTimelineValue(stderrText)
		emitStructuredCommandEvent(onEvent, "structured_patch_apply_failed", "Structured patch apply failed", details)
		return err
	}
	details["stdout"] = truncateStructuredTimelineValue(stdoutText)
	emitStructuredCommandEvent(onEvent, "structured_patch_apply_finished", "Structured patch apply finished", details)
	return nil
}

func appendCachedStructuredCommandObservation(step, attempt int, commandID, command, workingDirectory, root string, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	if !commandCacheEligible(command) {
		return false, nil
	}
	index, err := BuildWorkspaceIndex(workingDirectory, 0)
	if err != nil {
		return false, err
	}
	key := CommandCacheKey(index, command)
	cacheRoot := commandCacheRootOrDefault(root, index.Workspace)
	entry, ok, err := LoadCommandCacheEntry(cacheRoot, key)
	if err != nil || !ok {
		return false, err
	}
	if entry.Command != strings.TrimSpace(command) || entry.InputHash != CommandCacheInputHash(index) {
		return false, nil
	}
	if entry.Stdout != "" {
		_, _ = io.WriteString(stdout, entry.Stdout)
	}
	if entry.Stderr != "" {
		_, _ = io.WriteString(stderr, entry.Stderr)
	}
	result.Command = command
	result.ExitCode = entry.ExitCode
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:       step,
		CommandID:  commandID,
		Command:    command,
		ExitCode:   entry.ExitCode,
		Stdout:     truncateStructuredObservation(entry.Stdout),
		Stderr:     truncateStructuredObservation(entry.Stderr),
		Cached:     true,
		CWD:        structuredPromptWorkingDirectory(workingDirectory),
		Attempt:    attempt,
		StartedAt:  entry.CreatedAt,
		FinishedAt: entry.CreatedAt,
	})
	emitStructuredCommandEvent(onEvent, "command_cache_hit", "Reused cached command observation for unchanged workspace inputs", map[string]string{
		"step":       fmt.Sprintf("%d", step),
		"command_id": commandID,
		"attempt":    fmt.Sprintf("%d", attempt),
		"command":    truncateStructuredTimelineValue(command),
		"cwd":        structuredPromptWorkingDirectory(workingDirectory),
		"exit_code":  fmt.Sprintf("%d", entry.ExitCode),
		"stdout":     structuredTimelineCommandOutput(entry.Stdout),
		"stderr":     structuredTimelineCommandOutput(entry.Stderr),
		"cached":     "true",
	})
	return true, nil
}

func structuredTimelineCommandOutput(raw string) string {
	trimmed := strings.TrimRight(raw, "\n")
	if strings.TrimSpace(trimmed) == "" {
		return "(empty)"
	}
	if len(trimmed) <= defaultStructuredObservationChars {
		return trimmed
	}
	return trimmed[:defaultStructuredObservationChars] + "\n[truncated]"
}

func saveStructuredCommandCache(command, workingDirectory, root string, exitCode int, stdout, stderr string, onEvent func(StructuredCommandEvent)) error {
	if !commandCacheEligible(command) {
		return nil
	}
	if exitCode != 0 {
		emitStructuredCommandEvent(onEvent, "command_cache_skipped", "Command observation was not cached because it failed", map[string]string{
			"command":   truncateStructuredTimelineValue(command),
			"exit_code": fmt.Sprintf("%d", exitCode),
		})
		return nil
	}
	index, err := BuildWorkspaceIndex(workingDirectory, 0)
	if err != nil {
		return err
	}
	key := CommandCacheKey(index, command)
	entry := CommandCacheEntry{
		Key:       key,
		Workspace: index.Workspace,
		Command:   strings.TrimSpace(command),
		InputHash: CommandCacheInputHash(index),
		ExitCode:  exitCode,
		Stdout:    truncateStructuredObservation(stdout),
		Stderr:    truncateStructuredObservation(stderr),
	}
	if err := SaveCommandCacheEntry(commandCacheRootOrDefault(root, index.Workspace), entry); err != nil {
		return err
	}
	emitStructuredCommandEvent(onEvent, "command_cache_stored", "Stored command observation for unchanged-input reuse", map[string]string{
		"command": truncateStructuredTimelineValue(command),
		"key":     key,
	})
	return nil
}

func commandCacheRootOrDefault(root, workspace string) string {
	if strings.TrimSpace(root) != "" {
		return root
	}
	return filepath.Join(workspace, ".omni", "command-cache")
}

func commandCacheEligible(command string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "go":
		return len(fields) >= 2 && fields[1] == "test"
	case "npm":
		return len(fields) >= 2 && (fields[1] == "test" || (fields[1] == "run" && len(fields) >= 3 && (fields[2] == "test" || fields[2] == "build")))
	case "git":
		return len(fields) >= 2 && (fields[1] == "status" || fields[1] == "diff" || fields[1] == "branch")
	case "test":
		return len(fields) >= 3 && fields[1] == "-f"
	default:
		return false
	}
}
