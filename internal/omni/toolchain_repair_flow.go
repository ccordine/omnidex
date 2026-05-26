package omni

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func upsertToolchainRepairChildJob(jobs []ChildJob, feedback ToolchainFeedback, obs StructuredCommandObservation, workingDir string) ([]ChildJob, ChildJob) {
	job := toolchainRepairChildJob(feedback, obs, workingDir)
	index := -1
	for i, existing := range jobs {
		if existing.ID == job.ID {
			index = i
			break
		}
	}
	if index >= 0 {
		existing := jobs[index]
		existing.Status = ChildJobStatusRepairing
		existing.Goal = firstNonEmpty(existing.Goal, job.Goal)
		existing.ScopeFiles = cleanStringList(append(existing.ScopeFiles, job.ScopeFiles...))
		existing.RequiredEvidencePredicates = cleanStringList(append(existing.RequiredEvidencePredicates, job.RequiredEvidencePredicates...))
		existing.ProofCommands = cleanStringList(append(existing.ProofCommands, job.ProofCommands...))
		if !childJobAttemptAlreadyRecorded(existing, obs) {
			existing = AppendChildJobAttemptWithContext(existing, obs, "runtime", "toolchain_feedback", feedback.Toolchain, workingDir)
		} else if existing.LatestFailurePacket == nil {
			existing.LatestFailurePacket = failurePacketFromObservation(existing, obs, classifyChildJobFailureKind(obs), workingDir)
		}
		jobs[index] = existing
		return jobs, existing
	}
	job = AppendChildJobAttemptWithContext(job, obs, "runtime", "toolchain_feedback", feedback.Toolchain, workingDir)
	jobs = append(jobs, job)
	return jobs, job
}

func toolchainRepairChildJob(feedback ToolchainFeedback, obs StructuredCommandObservation, workingDir string) ChildJob {
	id := toolchainRepairChildJobID(feedback)
	command := strings.TrimSpace(firstNonEmpty(feedback.Command, obs.Command))
	scope := toolchainFeedbackScopeFiles(feedback, obs, workingDir)
	predicates := []string{}
	if command != "" {
		predicates = append(predicates, "command_passed:"+command)
	}
	if feedback.Toolchain == "vite" && strings.Contains(strings.ToLower(feedback.Summary+" "+strings.Join(feedback.Hints, " ")), "jsx") {
		predicates = append([]string{"no_js_files_with_jsx:src"}, predicates...)
	}
	return ChildJob{
		ID:                         id,
		ParentObjectiveID:          "toolchain_feedback_repair",
		Goal:                       firstNonEmpty(feedback.Summary, "Repair classified toolchain failure"),
		Status:                     ChildJobStatusRepairing,
		ScopeFiles:                 scope,
		RequiredEvidencePredicates: cleanStringList(predicates),
		ProofCommands:              cleanStringList([]string{command}),
	}
}

func toolchainRepairChildJobID(feedback ToolchainFeedback) string {
	toolchain := strings.TrimSpace(feedback.Toolchain)
	if toolchain == "" {
		toolchain = "toolchain"
	}
	kind := strings.TrimSpace(feedback.Kind)
	if kind == "" {
		kind = "failure"
	}
	return "repair_" + slugTag(toolchain+"_"+kind)
}

func toolchainFeedbackScopeFiles(feedback ToolchainFeedback, obs StructuredCommandObservation, workingDir string) []string {
	text := obs.Stdout + "\n" + obs.Stderr + "\n" + feedback.Summary + "\n" + strings.Join(feedback.Hints, "\n")
	files := []string{}
	for _, token := range strings.Fields(strings.NewReplacer("\n", " ", "\r", " ", "\t", " ", "(", " ", ")", " ", "[", " ", "]", " ", "\"", " ", "'", " ").Replace(text)) {
		clean := strings.Trim(token, ":,;")
		slash := filepath.ToSlash(clean)
		if idx := strings.Index(slash, "src/"); idx >= 0 {
			slash = slash[idx:]
		}
		for _, ext := range []string{".go", ".js", ".jsx", ".ts", ".tsx", ".rs", ".zig"} {
			if extIdx := strings.Index(strings.ToLower(slash), ext); extIdx >= 0 {
				slash = slash[:extIdx+len(ext)]
				break
			}
		}
		if slash == "" || strings.Contains(slash, " ") {
			continue
		}
		if strings.Contains(slash, "/") && isCodeContextFile(slash) {
			files = append(files, slash)
		}
	}
	if len(files) == 0 && strings.TrimSpace(workingDir) != "" {
		files = append(files, ".")
	}
	return cleanStringList(files)
}

func emitToolchainRepairControlFlowEvents(step int, feedback ToolchainFeedback, job ChildJob, onEvent func(StructuredCommandEvent)) {
	emitStructuredCommandEvent(onEvent, "toolchain_repair_child_job_created", "Toolchain feedback created or updated active repair child job", map[string]string{
		"step":             fmt.Sprintf("%d", step),
		"repair_child_job": job.ID,
		"toolchain":        feedback.Toolchain,
		"kind":             feedback.Kind,
	})
	emitStructuredCommandEvent(onEvent, "toolchain_repair_focus_locked", "Toolchain repair focus locked until child job evidence passes", map[string]string{
		"step":             fmt.Sprintf("%d", step),
		"active_child_job": job.ID,
		"required_evidence": strings.Join(job.RequiredEvidencePredicates, ","),
	})
	emitStructuredCommandEvent(onEvent, "child_job_next_action_derived", "Next action routed to active toolchain repair child job", map[string]string{
		"step":         fmt.Sprintf("%d", step),
		"child_job_id": job.ID,
		"action_kind":  "patch",
	})
}

func routeActiveToolchainRepairChildBeforePlanner(ctx context.Context, step int, prompt string, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc, result *CommandDecisionResult) (bool, error) {
	if result == nil {
		return false, nil
	}
	index := activeToolchainRepairChildJobIndex(result.ChildJobs)
	if result.TaskMode == TaskModeResearchOnly {
		if index >= 0 {
			result.ChildJobs = removeActiveToolchainRepairChildJobs(result.ChildJobs)
			emitStructuredCommandEvent(onEvent, "research_only_repair_jobs_suppressed", "Research-only mode suppressed stale toolchain repair child jobs", map[string]string{
				"step": fmt.Sprintf("%d", step),
			})
		}
		return false, nil
	}
	if index < 0 {
		return false, nil
	}
	job := result.ChildJobs[index]
	emitStructuredCommandEvent(onEvent, "toolchain_repair_focus_lock_active", "Generic planner skipped while toolchain repair child job is active", map[string]string{
		"step":             fmt.Sprintf("%d", step),
		"active_child_job": job.ID,
	})
	if cfg.ShellSpecialist == nil {
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "toolchain_repair_focus_lock: active repair child job requires focused repair before generic planner can continue",
		})
		if result.ExitCode == 0 {
			result.ExitCode = 1
		}
		return true, CommandDecisionExhaustedError{MaxSteps: step}
	}
	toolTask := toolchainRepairToolTask(job)
	proposal, ok, err := proposeValidatedShellCommand(ctx, step, prompt, toolTask, cfg, worksiteSurvey, &result.ObjectiveLedger, onEvent, onAsk, result)
	if err != nil || !ok {
		return true, err
	}
	if err := runDelegatedShellProposalWithLocalRepair(ctx, step, prompt, toolTask, proposal, cfg, worksiteSurvey, &result.ObjectiveLedger, stdout, stderr, onEvent, onAsk, result); err != nil {
		return true, err
	}
	return true, nil
}

func removeActiveToolchainRepairChildJobs(jobs []ChildJob) []ChildJob {
	if len(jobs) == 0 {
		return jobs
	}
	filtered := jobs[:0]
	for _, job := range jobs {
		if !childJobTerminal(job) && strings.HasPrefix(job.ID, "repair_") && job.ParentObjectiveID == "toolchain_feedback_repair" {
			continue
		}
		filtered = append(filtered, job)
	}
	return filtered
}

func activeToolchainRepairChildJobIndex(jobs []ChildJob) int {
	for i, job := range jobs {
		if childJobTerminal(job) {
			continue
		}
		if strings.HasPrefix(job.ID, "repair_") && job.ParentObjectiveID == "toolchain_feedback_repair" {
			return i
		}
	}
	return -1
}

func toolchainRepairToolTask(job ChildJob) string {
	packet := ""
	if job.LatestFailurePacket != nil {
		packet = " FailurePacket: " + job.LatestFailurePacket.FailureKind + " stderr=" + truncateStructuredTimelineValue(job.LatestFailurePacket.StderrExcerpt)
	}
	return strings.TrimSpace("Active repair child job: " + job.ID +
		". Required next behavior: repair only this classified compiler/test failure before unrelated objectives. " +
		"Scope files: " + strings.Join(job.ScopeFiles, ",") +
		". Required evidence: " + strings.Join(job.RequiredEvidencePredicates, ",") +
		"." + packet)
}
