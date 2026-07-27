package omni

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func selectedExternalArchitectAgent(cfg structuredCommandDecisionRunConfig) (CursorArchitectAgent, string) {
	agent, name, _ := selectedAvailableExternalArchitectAgent(cfg)
	return agent, name
}

func selectedAvailableExternalArchitectAgent(cfg structuredCommandDecisionRunConfig) (CursorArchitectAgent, string, string) {
	reasons := []string{}
	if cfg.CodexArchitectAgent != nil {
		if reason := externalArchitectAgentUnavailableReason(cfg.CodexArchitectAgent, "codex_sdk"); reason == "" {
			return cfg.CodexArchitectAgent, "codex_sdk", ""
		} else {
			reasons = append(reasons, reason)
		}
	}
	if cfg.CursorArchitectAgent != nil {
		if reason := externalArchitectAgentUnavailableReason(cfg.CursorArchitectAgent, "cursor_sdk"); reason == "" {
			return cfg.CursorArchitectAgent, "cursor_sdk", ""
		} else {
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "no external architect agent is enabled and selected")
	}
	return nil, "", strings.Join(reasons, "; ")
}

type externalArchitectAgentAvailability interface {
	ArchitectAgentAvailable() (bool, string)
}

func externalArchitectAgentUnavailableReason(agent CursorArchitectAgent, agentName string) string {
	if agent == nil {
		return agentName + " architect agent is not configured"
	}
	if availability, ok := agent.(externalArchitectAgentAvailability); ok {
		available, reason := availability.ArchitectAgentAvailable()
		if !available {
			reason = strings.TrimSpace(reason)
			if reason == "" {
				reason = agentName + " architect agent is unavailable"
			}
			return reason
		}
	}
	return ""
}

func emitExternalArchitectUnavailable(step int, reason string, onEvent func(StructuredCommandEvent)) {
	if strings.TrimSpace(reason) == "" {
		reason = "external architect agent is unavailable"
	}
	emitStructuredCommandEvent(onEvent, "external_agent_unavailable", "External architect agent unavailable; routing to local Omnidex actor", map[string]string{
		"step":   fmt.Sprintf("%d", step),
		"reason": truncateStructuredTimelineValue(reason),
	})
}

func shouldDelegateArchitectContractToExternalAgent(contract ImplementationArchitectContract, observations []StructuredCommandObservation) bool {
	if !hasImplementationArchitectContract(contract) || contract.CurrentItem == nil {
		return false
	}
	if strings.HasPrefix(strings.TrimSpace(contract.CurrentItem.ID), "read_before_") {
		return false
	}
	for _, obs := range observations {
		if obs.ExitCode == 0 && (strings.HasPrefix(strings.TrimSpace(obs.Command), "cursor_sdk.architect_agent ") || strings.HasPrefix(strings.TrimSpace(obs.Command), "codex_sdk.architect_agent ")) {
			return false
		}
		if obs.ExitCode != 0 && (strings.HasPrefix(strings.TrimSpace(obs.Command), "cursor_sdk.architect_agent ") || strings.HasPrefix(strings.TrimSpace(obs.Command), "codex_sdk.architect_agent ")) {
			return false
		}
	}
	return true
}

func runExternalArchitectAgentLane(ctx context.Context, step int, prompt, toolTask string, contract ImplementationArchitectContract, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult, agent CursorArchitectAgent, agentName string) (bool, error) {
	if agentName == "" {
		agentName = "external_sdk"
	}
	if agent == nil {
		return true, fmt.Errorf("%s architect agent is nil", agentName)
	}
	if result == nil {
		return true, fmt.Errorf("%s architect result target is nil", agentName)
	}
	if contract.CurrentItem == nil {
		return true, fmt.Errorf("%s architect contract is missing current_item", agentName)
	}
	emitStructuredCommandEvent(onEvent, agentName+"_architect_agent_started", "Implementation architect delegated bounded implementation packet to external coding agent", map[string]string{
		"step":        fmt.Sprintf("%d", step),
		"target_root": contract.TargetRoot,
		"current":     contract.CurrentItem.ID,
	})
	agentInput := CursorArchitectAgentInput{
		Step:              step,
		UserPrompt:        prompt,
		ToolTask:          toolTask,
		ArchitectContract: contract,
		Packet:            buildCursorImplementationPacket(prompt, toolTask, contract, cfg, worksiteSurvey),
		Observations:      compactStructuredObservationsForContext(result.Observations, 8, 800),
		SessionMemories:   compactSessionMemoriesForStructuredContext(cfg.SessionMemories, 6, 600),
		WorksiteSurvey:    worksiteSurvey,
		Workspace:         cfg.CurrentWorkingDirectory,
	}
	agentResult, err := runExternalArchitectAgentTask(ctx, agent, agentName, agentInput, onEvent)
	if err != nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			Command:  agentName + ".architect_agent " + contract.TargetRoot,
			ExitCode: 1,
			Stderr:   err.Error(),
		})
		emitStructuredCommandEvent(onEvent, agentName+"_architect_agent_failed", "External coding agent failed; no alternate implementation was started", map[string]string{
			"step":  fmt.Sprintf("%d", step),
			"error": truncateStructuredTimelineValue(err.Error()),
		})
		return true, fmt.Errorf("%s architect agent failed: %w", agentName, err)
	}
	command := agentName + ".architect_agent " + contract.TargetRoot
	result.Command = command
	result.ExitCode = 0
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:         step,
		Command:      command,
		EvidenceKind: "implementation",
		GeneratedBy:  agentName,
		ExitCode:     0,
		Stdout:       truncateStructuredTimelineValue(firstNonEmpty(agentResult.Summary, agentResult.Output, "external coding agent completed")),
	})
	emitStructuredCommandEvent(onEvent, agentName+"_architect_agent_completed", "External coding agent completed delegated implementation", map[string]string{
		"step":     fmt.Sprintf("%d", step),
		"agent_id": agentResult.AgentID,
		"run_id":   agentResult.RunID,
		"summary":  truncateStructuredTimelineValue(firstNonEmpty(agentResult.Summary, agentResult.Output)),
	})
	appendCursorArchitectFileObservations(step, contract, cfg.CurrentWorkingDirectory, result)
	for _, proof := range contract.ProofCommands {
		verifyCommand := commandInArchitectCWD(contract.TargetRoot, proof)
		if strings.TrimSpace(verifyCommand) == "" {
			continue
		}
		emitStructuredCommandEvent(onEvent, agentName+"_architect_validation_started", "Running proof command after external coding agent", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"command": truncateStructuredTimelineValue(verifyCommand),
		})
		if err := runStructuredPayloadCommand(ctx, step, verifyCommand, cfg.CurrentWorkingDirectory, cfg.EnableCommandCache, cfg.CommandCacheRoot, stdout, stderr, onEvent, result); err != nil {
			return true, err
		}
		if result.ExitCode != 0 {
			emitStructuredCommandEvent(onEvent, agentName+"_architect_validation_failed", "Proof command failed after external coding agent", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"command": truncateStructuredTimelineValue(verifyCommand),
				"stderr":  truncateStructuredTimelineValue(latestFailedCommandOutput(result.Observations, verifyCommand)),
			})
			return true, nil
		}
	}
	emitStructuredCommandEvent(onEvent, agentName+"_architect_validation_passed", "External coding agent output passed configured proof commands", map[string]string{
		"step": fmt.Sprintf("%d", step),
	})
	return true, nil
}

func runExternalArchitectAgentTask(ctx context.Context, agent CursorArchitectAgent, agentName string, input CursorArchitectAgentInput, onEvent func(StructuredCommandEvent)) (CursorArchitectAgentResult, error) {
	if provider, ok := agent.(ExternalAgentSessionProvider); ok {
		session, err := provider.NewExternalAgentSession(input)
		if err != nil {
			return CursorArchitectAgentResult{}, err
		}
		job := ExternalAgentJob{
			SessionID: agentName + "-architect",
			Agent:     strings.TrimSuffix(agentName, "_sdk"),
			Mode:      "implementation",
			Packet:    input.Packet,
			Prompt:    externalAgentPromptForName(agentName, input),
			Workspace: input.Workspace,
		}
		return StreamExternalAgentSession(ctx, session, job, func(event AgentEvent) error {
			structured := structuredEventFromExternalAgentEvent(event)
			emitStructuredCommandEvent(onEvent, structured.Type, structured.Summary, structured.Details)
			return nil
		})
	}
	return agent.RunArchitectTask(ctx, input)
}

func externalAgentPromptForName(agentName string, input CursorArchitectAgentInput) string {
	if strings.HasPrefix(agentName, "codex") {
		return buildCodexArchitectPrompt(input)
	}
	return buildCursorArchitectPrompt(input)
}

func runCursorArchitectAgentLane(ctx context.Context, step int, prompt, toolTask string, contract ImplementationArchitectContract, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	return runExternalArchitectAgentLane(ctx, step, prompt, toolTask, contract, cfg, worksiteSurvey, stdout, stderr, onEvent, result, cfg.CursorArchitectAgent, "cursor_sdk")
}

func buildCursorImplementationPacket(prompt, toolTask string, contract ImplementationArchitectContract, cfg structuredCommandDecisionRunConfig, survey WorksiteSurvey) CursorImplementationPacket {
	targetRoot := firstNonEmpty(strings.TrimSpace(contract.TargetRoot), ".")
	packet := CursorImplementationPacket{
		Task:       strings.TrimSpace(prompt),
		Mode:       "implementation_only",
		Workspace:  strings.TrimSpace(cfg.CurrentWorkingDirectory),
		TargetRoot: targetRoot,
		Worksite: CursorPacketWorksite{
			ProjectState:   survey.ProjectState,
			PackageManager: firstNonEmpty(contract.PackageManager, survey.PackageManager),
			Frameworks:     survey.Frameworks,
		},
		EditSurface:     cursorPacketEditSurface(contract),
		ReadOnlyContext: cursorPacketReadOnlyContext(contract),
		Objectives:      cursorPacketObjectives(prompt, toolTask, contract),
		ProofContract: CursorPacketProofContract{
			Commands:           cursorPacketProofCommands(contract),
			ArtifactChecks:     cursorPacketArtifactChecks(contract),
			EvidencePredicates: cursorPacketEvidencePredicates(contract),
		},
		Forbidden: cursorPacketForbiddenActions(contract),
		ReturnRequirements: []string{
			"files changed",
			"implementation summary",
			"commands run if any",
			"known risks",
		},
		PreparedContext: cursorPacketPreparedContext(cfg.PrepContext, cfg.SessionMemories),
	}
	if packet.Task == "" {
		packet.Task = strings.TrimSpace(toolTask)
	}
	return packet
}

func cursorPacketEditSurface(contract ImplementationArchitectContract) []string {
	out := []string{}
	for _, path := range contract.EditSurface {
		if strings.TrimSpace(path) != "" {
			out = append(out, filepath.ToSlash(path))
		}
	}
	for _, item := range contract.WorkQueue {
		if (item.Operation == "create" || item.Operation == "update" || item.Operation == "delete") && strings.TrimSpace(item.Path) != "" {
			out = append(out, filepath.ToSlash(filepath.Join(item.CWD, item.Path)))
		}
	}
	return appendUniqueStrings(nil, out...)
}

func cursorPacketReadOnlyContext(contract ImplementationArchitectContract) []string {
	out := []string{}
	for _, item := range contract.WorkQueue {
		if item.Operation == "read" && strings.TrimSpace(item.Path) != "" {
			out = append(out, filepath.ToSlash(filepath.Join(item.CWD, item.Path)))
		}
	}
	for _, path := range []string{"package.json", "src/main.jsx", "src/main.js", "src/index.js", "vite.config.js"} {
		out = append(out, filepath.ToSlash(filepath.Join(contract.TargetRoot, path)))
	}
	edit := map[string]bool{}
	for _, path := range cursorPacketEditSurface(contract) {
		edit[path] = true
	}
	filtered := []string{}
	for _, path := range appendUniqueStrings(nil, out...) {
		if !edit[path] {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func cursorPacketObjectives(prompt, toolTask string, contract ImplementationArchitectContract) []string {
	out := []string{}
	out = append(out, contract.AcceptanceCriteria...)
	for _, item := range contract.WorkQueue {
		if strings.TrimSpace(item.Description) != "" && (item.Operation == "create" || item.Operation == "update") {
			out = append(out, item.Description)
		}
	}
	if len(out) == 0 {
		out = append(out, strings.TrimSpace(firstNonEmpty(prompt, toolTask)))
	}
	return appendUniqueStrings(nil, out...)
}

func cursorPacketProofCommands(contract ImplementationArchitectContract) []string {
	out := []string{}
	for _, command := range contract.ProofCommands {
		if strings.TrimSpace(command) != "" {
			out = append(out, commandInArchitectCWD(contract.TargetRoot, command))
		}
	}
	for _, item := range contract.WorkQueue {
		if item.Operation == "verify" && strings.TrimSpace(item.Verify) != "" {
			out = append(out, commandInArchitectCWD(item.CWD, item.Verify))
		}
	}
	return appendUniqueStrings(nil, out...)
}

func cursorPacketArtifactChecks(contract ImplementationArchitectContract) []string {
	checks := []string{
		"created source/test/config files are non-empty",
		"created source/test/config files are not placeholder-only",
		"package scripts remain valid if package.json is changed",
		"do not delete or weaken validated tests",
	}
	for _, path := range cursorPacketEditSurface(contract) {
		checks = append(checks, "edited file remains within edit surface: "+path)
	}
	return appendUniqueStrings(nil, checks...)
}

func cursorPacketEvidencePredicates(contract ImplementationArchitectContract) []string {
	out := []string{"artifact_validation_passed"}
	for _, command := range cursorPacketProofCommands(contract) {
		out = append(out, "command_passed:"+command)
	}
	for _, item := range contract.WorkQueue {
		if (item.Operation == "create" || item.Operation == "update") && strings.TrimSpace(item.Path) != "" {
			out = append(out, "file_nonempty:"+filepath.ToSlash(filepath.Join(item.CWD, item.Path)))
		}
	}
	return appendUniqueStrings(nil, out...)
}

func cursorPacketForbiddenActions(contract ImplementationArchitectContract) []string {
	out := append([]string{}, contract.Guardrails...)
	out = append(out,
		"do not create a sibling project",
		"do not push to git or perform release actions",
		"do not add unrequested dependencies",
		"do not add backend, database, auth, routing, or cloud sync unless explicitly requested",
		"do not delete, skip, or weaken validated tests",
		"do not claim objective completion; Omnidex will run proof commands and decide completion",
	)
	return appendUniqueStrings(nil, out...)
}

func cursorPacketPreparedContext(prep PrepContextBundle, memories []SessionMemory) []string {
	out := []string{}
	for _, file := range prep.CodebaseRoute.LikelyFiles {
		if strings.TrimSpace(file) != "" {
			out = append(out, "likely_file:"+file)
		}
	}
	for _, brief := range limitPrepBriefsForArchitect(allPrepBriefs(prep), 6) {
		out = append(out, brief.Kind+": "+truncateStructuredTimelineValue(brief.Content))
	}
	for _, memory := range compactSessionMemoriesForStructuredContext(memories, 4, 500) {
		out = append(out, "memory:"+truncateStructuredTimelineValue(memory.Content))
	}
	return appendUniqueStrings(nil, out...)
}

func appendCursorArchitectFileObservations(step int, contract ImplementationArchitectContract, workingDir string, result *CommandDecisionResult) {
	for _, item := range contract.WorkQueue {
		switch item.Operation {
		case "read":
			if strings.TrimSpace(item.Path) == "" || strings.HasSuffix(item.Path, "/") {
				continue
			}
			if _, err := os.Stat(filepath.Join(workingDir, item.CWD, item.Path)); err == nil {
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					Command:  "architect.read " + filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
					ExitCode: 0,
					Stdout:   "read delegated to Cursor SDK architect agent",
				})
			}
		case "create", "update":
			if strings.TrimSpace(item.Path) == "" || strings.HasSuffix(item.Path, "/") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(workingDir, item.CWD, item.Path))
			if err != nil || strings.TrimSpace(string(content)) == "" {
				continue
			}
			if err := validateCodeContentProposalForArchitectItem(string(content), contract, item); err != nil {
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:            step,
					RejectedCommand: "cursor_sdk.architect_agent " + filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
					ExitCode:        1,
					Stderr:          err.Error(),
				})
				continue
			}
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				Command:  fmt.Sprintf("architect.apply %s %s", item.Operation, filepath.ToSlash(filepath.Join(item.CWD, item.Path))),
				ExitCode: 0,
				Stdout:   "file produced by Cursor SDK architect agent",
			})
		}
	}
}

func blockShellFallbackForArchitectFileWork(step int, prompt, toolTask string, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	contract := enrichImplementationArchitectContract(buildImplementationArchitectContract(prompt, toolTask, cfg.CurrentWorkingDirectory, worksiteSurvey, result.Observations), prompt, toolTask, cfg.PrepContext, cfg.SessionMemories)
	if !hasImplementationArchitectContract(contract) || !architectItemRequiresCodeOwnedFileWork(contract.CurrentItem) {
		return false
	}
	item := *contract.CurrentItem
	path := filepath.ToSlash(filepath.Join(item.CWD, item.Path))
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		ExitCode: 1,
		Stderr:   "code-owned architect file work item " + item.ID + " requires deterministic architect/coder execution for exact path " + path + "; shell specialist path selection is disabled",
	})
	emitStructuredCommandEvent(onEvent, "structured_tool_delegation_blocked_for_code_owned_file", "Shell specialist bypassed for code-owned architect file work", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"item_id":   item.ID,
		"operation": item.Operation,
		"path":      path,
	})
	return true
}

func architectCurrentItemPending(prompt, toolTask string, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, observations []StructuredCommandObservation) bool {
	contract := enrichImplementationArchitectContract(buildImplementationArchitectContract(prompt, toolTask, cfg.CurrentWorkingDirectory, worksiteSurvey, observations), prompt, toolTask, cfg.PrepContext, cfg.SessionMemories)
	return hasImplementationArchitectContract(contract) && contract.CurrentItem != nil
}

func architectItemRequiresCodeOwnedFileWork(item *ArchitectWorkItem) bool {
	if item == nil || strings.TrimSpace(item.Path) == "" || strings.HasSuffix(strings.TrimSpace(item.Path), "/") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(item.Operation)) {
	case "create", "update", "delete", "read":
		return true
	default:
		return false
	}
}

func validateArchitectDeleteTarget(workingDir string, item ArchitectWorkItem, targetPath string) error {
	if strings.TrimSpace(item.Path) == "" {
		return fmt.Errorf("delete work item missing target path")
	}
	root := filepath.Clean(filepath.Join(workingDir, item.CWD))
	target := filepath.Clean(targetPath)
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return fmt.Errorf("delete target safety validation failed: %w", err)
	}
	if rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." || filepath.IsAbs(rel) {
		return fmt.Errorf("delete target %q escapes architect root %q", target, root)
	}
	if info, err := os.Stat(target); err != nil {
		return fmt.Errorf("delete target %q is not accessible: %w", target, err)
	} else if info.IsDir() {
		return fmt.Errorf("delete target %q is a directory; queued delete requires an exact file target", target)
	}
	return nil
}

func generateValidatedCodeContent(ctx context.Context, step int, prompt string, contract ImplementationArchitectContract, item ArchitectWorkItem, existing string, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (CodeContentProposal, error) {
	var proposal CodeContentProposal
	var err error
	var lastRejectedProposal CodeContentProposal
	rejectedContent := map[string]struct{}{}
	emitStructuredCommandEvent(onEvent, "architect_work_item_repair_started", "Implementation architect entered focused work-item repair loop", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"item_id": item.ID,
		"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
		"status":  "generating",
	})
	for attempt := 0; attempt <= defaultShellSpecialistRepairAttempts; attempt++ {
		if attempt > 0 {
			emitStructuredCommandEvent(onEvent, "architect_work_item_repair_feedback", "Code content specialist received direct validator feedback for focused work item", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
				"attempt": fmt.Sprintf("%d", attempt),
				"reason":  truncateStructuredTimelineValue(latestCodeContentRepairFeedback(result.Observations)),
			})
		}
		emitStructuredCommandEvent(onEvent, "architect_work_item_repair_attempted", "Implementation architect attempted code content for focused work item", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"item_id": item.ID,
			"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
			"attempt": fmt.Sprintf("%d", attempt+1),
			"status":  "generating",
		})
		proposal, err = cfg.CodeContentSpecialist.GenerateCodeContent(ctx, CodeContentSpecialistInput{
			Step:               step,
			UserPrompt:         prompt,
			ArchitectContract:  contract,
			WorkItem:           item,
			ExistingContent:    existing,
			TestFirst:          architectWorkItemIsTestFirst(item),
			RepairFeedback:     latestCodeContentRepairFeedback(result.Observations),
			RepairAttempt:      attempt,
			RejectedContent:    lastRejectedProposal.Content,
			IntegrationContext: integrationContextForProjectFileEntry(contract.ProjectFileMap, item.Path),
			ProjectFileMap:     contract.ProjectFileMap,
			Observations:       result.Observations,
			SessionMemories:    cfg.SessionMemories,
			WorksiteSurvey:     worksiteSurvey,
		})
		if err != nil {
			return proposal, err
		}
		contentFingerprint := sha256String(strings.TrimSpace(proposal.Content))
		if _, repeated := rejectedContent[contentFingerprint]; repeated {
			err := fmt.Errorf("repeated rejected content for architect work item %s", item.ID)
			appendRejectedCodeContentObservation(step, item, err, result)
			emitStructuredCommandEvent(onEvent, "architect_work_item_repair_repeated_content_rejected", "Implementation architect rejected repeated invalid content for focused work item", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
				"attempt": fmt.Sprintf("%d", attempt+1),
				"status":  "rejected",
			})
			if attempt < defaultShellSpecialistRepairAttempts {
				continue
			}
			return failRejectedArchitectItem(step, item, err, onEvent)
		}
		if err := validateCodeContentProposalForArchitectItem(proposal.Content, contract, item); err != nil {
			rejectedContent[contentFingerprint] = struct{}{}
			lastRejectedProposal = proposal
			appendRejectedCodeContentObservation(step, item, err, result)
			emitStructuredCommandEvent(onEvent, "architect_work_item_content_rejected", "Code content rejected by architect-scoped validator", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
				"reason":  truncateStructuredTimelineValue(err.Error()),
			})
			if isArchitectContentKindValidationError(err) {
				emitStructuredCommandEvent(onEvent, "architect_work_item_content_kind_rejected", "Code content rejected because it did not match the target file kind", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"item_id": item.ID,
					"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
					"reason":  truncateStructuredTimelineValue(err.Error()),
				})
			}
			emitStructuredCommandEvent(onEvent, "architect_work_item_repair_rejected", "Implementation architect repair attempt failed validation for focused work item", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
				"attempt": fmt.Sprintf("%d", attempt+1),
				"reason":  truncateStructuredTimelineValue(err.Error()),
				"status":  "rejected",
			})
			if attempt < defaultShellSpecialistRepairAttempts {
				continue
			}
			return failRejectedArchitectItem(step, item, err, onEvent)
		}
		if err := validateArchitectContentAlignsWithPrompt(proposal.Content, item, prompt, contract); err != nil {
			rejectedContent[contentFingerprint] = struct{}{}
			lastRejectedProposal = proposal
			appendRejectedCodeContentObservation(step, item, err, result)
			emitStructuredCommandEvent(onEvent, "architect_work_item_alignment_rejected", "Code content rejected because it did not align with the active prompt", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
				"reason":  truncateStructuredTimelineValue(err.Error()),
			})
			emitStructuredCommandEvent(onEvent, "architect_work_item_repair_rejected", "Implementation architect repair attempt failed validation for focused work item", map[string]string{
				"step":    fmt.Sprintf("%d", step),
				"item_id": item.ID,
				"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
				"attempt": fmt.Sprintf("%d", attempt+1),
				"reason":  truncateStructuredTimelineValue(err.Error()),
				"status":  "rejected",
			})
			if attempt < defaultShellSpecialistRepairAttempts {
				continue
			}
			return failRejectedArchitectItem(step, item, err, onEvent)
		}
		return proposal, nil
	}
	return proposal, nil
}

func failRejectedArchitectItem(step int, item ArchitectWorkItem, rejection error, onEvent func(StructuredCommandEvent)) (CodeContentProposal, error) {
	emitStructuredCommandEvent(onEvent, "architect_work_item_failed_with_evidence", "Implementation architect exhausted focused repair attempts", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"item_id": item.ID,
		"path":    filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
		"reason":  truncateStructuredTimelineValue(rejection.Error()),
		"status":  "failed",
	})
	return CodeContentProposal{}, fmt.Errorf("architect work item %s rejected after %d repair attempts: %w", item.ID, defaultShellSpecialistRepairAttempts+1, rejection)
}

func sha256String(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum)
}

func latestCodeContentRepairFeedback(observations []StructuredCommandObservation) string {
	return latestStructuredRepairFeedback(observations)
}

func guidanceForRejectedCodeContent(err error) string {
	if err == nil {
		return "repair the generated file content to satisfy file_contract and validator feedback"
	}
	text := strings.ToLower(err.Error())
	switch {
	case strings.Contains(text, "content kind rejected"):
		return "rewrite the file to match file_contract.language exactly; remove every forbidden content class listed in file_contract.must_avoid"
	case strings.Contains(text, "repeated rejected content"):
		return "previous content was identical and rejected again; produce materially different file content that satisfies file_contract"
	default:
		return "repair the generated file content to satisfy the validator feedback and file_contract"
	}
}

func appendRejectedCodeContentObservation(step int, item ArchitectWorkItem, err error, result *CommandDecisionResult) {
	if result == nil || err == nil {
		return
	}
	reason := err.Error()
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:            step,
		RejectedCommand: "architect.apply " + item.Operation + " " + filepath.ToSlash(filepath.Join(item.CWD, item.Path)),
		ExitCode:        1,
		Stderr:          "code content specialist content rejected: " + reason + "; " + guidanceForRejectedCodeContent(err),
	})
}
