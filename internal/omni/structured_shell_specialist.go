package omni

import (
	"context"
	"fmt"
	"strings"
)

func proposeValidatedShellCommand(ctx context.Context, step int, prompt, toolTask string, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, ledger *[]StructuredObjective, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc, result *CommandDecisionResult) (ShellCommandProposal, bool, error) {
	if cfg.ShellSpecialist == nil {
		return ShellCommandProposal{}, false, nil
	}
	architectContract := enrichImplementationArchitectContract(buildImplementationArchitectContract(prompt, toolTask, cfg.CurrentWorkingDirectory, worksiteSurvey, result.Observations), prompt, toolTask, cfg.PrepContext, cfg.SessionMemories)
	architectContract = enrichImplementationArchitectContractWithProjectMap(architectContract, cfg.CurrentWorkingDirectory, result.Observations)
	if hasImplementationArchitectContract(architectContract) {
		emitStructuredCommandEvent(onEvent, "implementation_architect_contract_created", "Implementation architect created coding contract", map[string]string{
			"step":           fmt.Sprintf("%d", step),
			"target_root":    architectContract.TargetRoot,
			"framework":      architectContract.Framework,
			"proof_commands": strings.Join(architectContract.ProofCommands, ","),
		})
		if len(architectContract.ProjectFileMap.Files) > 0 {
			activePath := ""
			if architectContract.ProjectFileMap.ActiveFile != nil {
				activePath = architectContract.ProjectFileMap.ActiveFile.Path
			}
			emitStructuredCommandEvent(onEvent, "project_file_map_built", "Implementation architect built project file map from work queue", map[string]string{
				"step":         fmt.Sprintf("%d", step),
				"file_count":   fmt.Sprintf("%d", len(architectContract.ProjectFileMap.Files)),
				"active_file":  activePath,
				"tree_summary": truncateStructuredTimelineValue(architectContract.ProjectFileMap.TreeSummary),
			})
		}
	}
	var lastRejectedCommand string
	for attempt := 0; attempt <= defaultShellSpecialistRepairAttempts; attempt++ {
		if attempt > 0 {
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_repair_started", "Shell specialist received direct validator feedback for local repair", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"attempt": fmt.Sprintf("%d", attempt),
			})
		}
		proposal, err := cfg.ShellSpecialist.ProposeShellCommand(ctx, ShellCommandSpecialistInput{
			Step:              step,
			UserPrompt:        prompt,
			ToolTask:          toolTask,
			ArchitectContract: architectContract,
			RepairFeedback:    latestShellRepairFeedback(result.Observations),
			RepairAttempt:     attempt,
			RejectedCommand:   lastRejectedCommand,
			Observations:      result.Observations,
			CompletedActions:  completedActionsFromState(*ledger, result.Observations),
			LoopState:         structuredLoopStateFromState(*ledger, result.Observations),
			ProjectFileMap:    architectContract.ProjectFileMap,
			SessionMemories:   cfg.SessionMemories,
			WorksiteSurvey:    worksiteSurvey,
		})
		if err != nil {
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_failed", "Shell specialist failed", map[string]string{
				"step":  fmt.Sprintf("%d", step),
				"error": truncateStructuredTimelineValue(err.Error()),
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "shell specialist failed: " + err.Error(),
			})
			return ShellCommandProposal{}, false, nil
		}
		proposal.Command = normalizeStructuredCommand(proposal.Command)
		emitStructuredCommandEvent(onEvent, "structured_tool_delegation_finished", "Shell specialist proposed command", map[string]string{
			"step":      fmt.Sprintf("%d", step),
			"tool":      "shell",
			"role":      "shell_execution_specialist",
			"attempt":   fmt.Sprintf("%d", attempt+1),
			"command":   truncateStructuredTimelineValue(proposal.Command),
			"rationale": truncateStructuredTimelineValue(proposal.Rationale),
		})
		if err := validateShellProposalDoesNotRepeatLatestFailedCommand(proposal.Command, result.Observations); err != nil {
			lastRejectedCommand = proposal.Command
			appendRejectedShellProposalObservation(step, proposal.Command, err, "use the observed failure as feedback and choose a different concrete command", result)
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Shell command rejected by execution feedback", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(proposal.Command),
				"reason":  err.Error(),
			})
			if attempt > 0 && shellProposalRepeatedLatestRejection(proposal.Command, result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_tool_delegation_repair_repeated", "Shell specialist repeated rejected command after direct feedback", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", attempt),
					"command": truncateStructuredTimelineValue(proposal.Command),
				})
				return ShellCommandProposal{}, false, nil
			}
			if attempt < defaultShellSpecialistRepairAttempts {
				continue
			}
			return ShellCommandProposal{}, false, nil
		}
		if err := validateShellProposalAgainstToolTaskWithRationale(proposal.Command, toolTask, proposal.Rationale); err != nil {
			if approved, askErr := requestDependencyInstallApproval(ctx, step, prompt, proposal.Command, err, cfg.UserAssistanceSpecialist, onAsk, onEvent, result); askErr != nil {
				return ShellCommandProposal{}, false, askErr
			} else if approved {
				return proposal, true, nil
			}
			appendRejectedShellProposalObservation(step, proposal.Command, err, "choose a write/edit/build/test command that directly satisfies the delegated task", result)
			lastRejectedCommand = proposal.Command
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Shell command rejected by tool-task constraints", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(proposal.Command),
				"reason":  err.Error(),
			})
			if attempt > 0 && shellProposalRepeatedLatestRejection(proposal.Command, result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_tool_delegation_repair_repeated", "Shell specialist repeated rejected command after direct feedback", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", attempt),
					"command": truncateStructuredTimelineValue(proposal.Command),
				})
				return ShellCommandProposal{}, false, nil
			}
			if attempt < defaultShellSpecialistRepairAttempts {
				continue
			}
			return ShellCommandProposal{}, false, nil
		}
		if err := validateCommandAgainstImplementationArchitectContract(proposal.Command, architectContract); err != nil {
			lastRejectedCommand = proposal.Command
			appendRejectedShellProposalObservation(step, proposal.Command, err, "choose a command that follows the implementation architect target root, edit surface, and proof commands", result)
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Shell command rejected by implementation architect contract", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(proposal.Command),
				"reason":  err.Error(),
			})
			if attempt > 0 && shellProposalRepeatedLatestRejection(proposal.Command, result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_tool_delegation_repair_repeated", "Shell specialist repeated rejected command after direct feedback", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", attempt),
					"command": truncateStructuredTimelineValue(proposal.Command),
				})
				return ShellCommandProposal{}, false, nil
			}
			if attempt < defaultShellSpecialistRepairAttempts {
				continue
			}
			return ShellCommandProposal{}, false, nil
		}
		if err := validateStructuredCommandForRunWithSurvey(proposal.Command, result.Observations, cfg.CurrentWorkingDirectory, *ledger, worksiteSurvey); err != nil {
			if approved, askErr := requestDependencyInstallApproval(ctx, step, prompt, proposal.Command, err, cfg.UserAssistanceSpecialist, onAsk, onEvent, result); askErr != nil {
				return ShellCommandProposal{}, false, askErr
			} else if approved {
				return proposal, true, nil
			}
			if handleStructuredRepeatedCommandValidation(step, proposal.Command, err, ledger, onEvent, result) {
				return ShellCommandProposal{}, false, nil
			}
			appendRejectedShellProposalObservation(step, proposal.Command, err, "planner should delegate a narrower shell task or choose a different tool", result)
			lastRejectedCommand = proposal.Command
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(proposal.Command),
				"reason":  err.Error(),
			})
			if attempt > 0 && shellProposalRepeatedLatestRejection(proposal.Command, result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_tool_delegation_repair_repeated", "Shell specialist repeated rejected command after direct feedback", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", attempt),
					"command": truncateStructuredTimelineValue(proposal.Command),
				})
				return ShellCommandProposal{}, false, nil
			}
			if attempt < defaultShellSpecialistRepairAttempts {
				continue
			}
			return ShellCommandProposal{}, false, nil
		}
		if attempt > 0 {
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_repair_accepted", "Shell specialist repaired proposal accepted by deterministic validation", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"attempt": fmt.Sprintf("%d", attempt),
				"command": truncateStructuredTimelineValue(proposal.Command),
			})
		}
		return proposal, true, nil
	}
	return ShellCommandProposal{}, false, nil
}

func shellProposalRepeatedLatestRejection(command string, observations []StructuredCommandObservation) bool {
	normalized := normalizeStructuredCommandForComparison(command)
	if normalized == "" {
		return false
	}
	count := 0
	for i := len(observations) - 1; i >= 0; i-- {
		if observations[i].RejectedCommand == "" {
			continue
		}
		if normalizeStructuredCommandForComparison(observations[i].RejectedCommand) == normalized {
			count++
			if count >= 2 {
				return true
			}
			continue
		}
		break
	}
	return false
}

func latestShellRepairFeedback(observations []StructuredCommandObservation) string {
	return latestStructuredRepairFeedback(observations)
}

func latestFailedCommandOutput(observations []StructuredCommandObservation, command string) string {
	normalizedCommand := normalizeStructuredCommandForComparison(command)
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode == 0 {
			continue
		}
		if normalizedCommand != "" && normalizeStructuredCommandForComparison(obs.Command) != normalizedCommand {
			continue
		}
		if text := strings.TrimSpace(obs.Stderr); text != "" {
			return truncateStructuredObservation(text)
		}
		if text := strings.TrimSpace(obs.Stdout); text != "" {
			return truncateStructuredObservation(text)
		}
	}
	for i := len(observations) - 1; i >= 0; i-- {
		obs := observations[i]
		if obs.ExitCode == 0 {
			continue
		}
		if text := strings.TrimSpace(obs.Stderr); text != "" {
			return truncateStructuredObservation(text)
		}
		if text := strings.TrimSpace(obs.Stdout); text != "" {
			return truncateStructuredObservation(text)
		}
	}
	return ""
}

func appendRejectedShellProposalObservation(step int, command string, err error, guidance string, result *CommandDecisionResult) {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:             step,
		RejectedCommand:  truncateStructuredObservation(command),
		CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(command, reason),
		ExitCode:         1,
		Stderr:           "shell specialist command rejected: " + reason + "; " + guidance,
	})
}
