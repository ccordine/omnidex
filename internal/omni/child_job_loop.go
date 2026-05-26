package omni

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

type ChildJobStatus string

const (
	ChildJobStatusPending             ChildJobStatus = "pending"
	ChildJobStatusActive              ChildJobStatus = "active"
	ChildJobStatusRepairing           ChildJobStatus = "repairing"
	ChildJobStatusVerifying           ChildJobStatus = "verifying"
	ChildJobStatusComplete            ChildJobStatus = "complete"
	ChildJobStatusFailedWithEvidence  ChildJobStatus = "failed_with_evidence"
	ChildJobStatusSkippedWithEvidence ChildJobStatus = "skipped_with_evidence"
	ChildJobStatusSuperseded          ChildJobStatus = "superseded"
)

type ChildJob struct {
	ID                         string                 `json:"id"`
	ParentObjectiveID          string                 `json:"parent_objective_id,omitempty"`
	Goal                       string                 `json:"goal"`
	Status                     ChildJobStatus         `json:"status"`
	ScopeFiles                 []string               `json:"scope_files,omitempty"`
	RequiredEvidencePredicates []string               `json:"required_evidence_predicates,omitempty"`
	ProofCommands              []string               `json:"proof_commands,omitempty"`
	AttemptLedger              []ChildJobAttempt      `json:"attempt_ledger,omitempty"`
	LatestFailurePacket        *FailurePacket         `json:"latest_failure_packet,omitempty"`
	ReviewerResult             ChildJobReviewerResult `json:"reviewer_result,omitempty"`
	TerminalReason             string                 `json:"terminal_reason,omitempty"`
}

type ChildJobAttempt struct {
	AttemptID       string    `json:"attempt_id"`
	Actor           string    `json:"actor,omitempty"`
	Role            string    `json:"role,omitempty"`
	Model           string    `json:"model,omitempty"`
	ActionKind      string    `json:"action_kind,omitempty"`
	CommandOrPatch  string    `json:"command_or_patch,omitempty"`
	Result          string    `json:"result"`
	FailureKind     string    `json:"failure_kind,omitempty"`
	ValidatorReason string    `json:"validator_reason,omitempty"`
	FullOutputRef   string    `json:"full_output_ref,omitempty"`
	ForbidRepeat    bool      `json:"forbid_repeat,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
}

type FailurePacket struct {
	ChildJobID              string   `json:"child_job_id,omitempty"`
	ObjectiveID             string   `json:"objective_id,omitempty"`
	FailedAction            string   `json:"failed_action,omitempty"`
	CommandID               string   `json:"command_id,omitempty"`
	CommandOrPatchSummary   string   `json:"command_or_patch_summary,omitempty"`
	CWD                     string   `json:"cwd,omitempty"`
	ExitCode                int      `json:"exit_code,omitempty"`
	StdoutExcerpt           string   `json:"stdout_excerpt,omitempty"`
	StderrExcerpt           string   `json:"stderr_excerpt,omitempty"`
	FailureKind             string   `json:"failure_kind,omitempty"`
	FailureFingerprint      string   `json:"failure_fingerprint,omitempty"`
	KnownState              string   `json:"known_state,omitempty"`
	RequiredEvidence        []string `json:"required_evidence,omitempty"`
	MissingEvidence         []string `json:"missing_evidence,omitempty"`
	ForbiddenNextActions    []string `json:"forbidden_next_actions,omitempty"`
	RequiredNextBehavior    string   `json:"required_next_behavior,omitempty"`
	AllowedNextActionKinds  []string `json:"allowed_next_action_kinds,omitempty"`
	FullOutputReferenceHint string   `json:"full_output_reference_hint,omitempty"`
}

type ChildJobReviewerResult struct {
	Accepted bool   `json:"accepted"`
	Reason   string `json:"reason,omitempty"`
	Reviewer string `json:"reviewer,omitempty"`
}

type ChildJobReviewInput struct {
	Job                  ChildJob                       `json:"job"`
	WorkingDirectory     string                         `json:"working_directory,omitempty"`
	Observations         []StructuredCommandObservation `json:"observations,omitempty"`
	PassedPredicates     []string                       `json:"passed_predicates,omitempty"`
	MissingPredicates    []string                       `json:"missing_predicates,omitempty"`
	DeterministicClosure bool                           `json:"deterministic_closure"`
}

type ChildJobReviewer interface {
	ReviewChildJob(input ChildJobReviewInput) ChildJobReviewerResult
}

type DeterministicChildJobReviewer struct{}

func (DeterministicChildJobReviewer) ReviewChildJob(input ChildJobReviewInput) ChildJobReviewerResult {
	if input.DeterministicClosure && len(input.MissingPredicates) == 0 {
		return ChildJobReviewerResult{Accepted: true, Reviewer: "deterministic_child_reviewer", Reason: "required evidence predicates passed"}
	}
	return ChildJobReviewerResult{Accepted: false, Reviewer: "deterministic_child_reviewer", Reason: "required evidence missing: " + strings.Join(input.MissingPredicates, ",")}
}

type ChildJobAction struct {
	Kind      string   `json:"kind"`
	JobID     string   `json:"job_id"`
	Command   string   `json:"command,omitempty"`
	Reason    string   `json:"reason,omitempty"`
	Scope     []string `json:"scope,omitempty"`
	Evidence  []string `json:"evidence,omitempty"`
	Forbidden []string `json:"forbidden,omitempty"`
}

type ChildJobEvent struct {
	Type    string            `json:"type"`
	Summary string            `json:"summary"`
	Details map[string]string `json:"details,omitempty"`
}

type ChildJobLoopInput struct {
	Jobs              []ChildJob
	WorkingDirectory  string
	Observations      []StructuredCommandObservation
	ObjectiveLedger   []StructuredObjective
	ProposedParentJob *ChildJob
	Reviewer          ChildJobReviewer
}

type ChildJobLoopResult struct {
	Jobs              []ChildJob
	ObjectiveLedger   []StructuredObjective
	ActiveJob         *ChildJob
	NextAction        *ChildJobAction
	PlannerBlocked    bool
	ParentJobDeferred bool
	Events            []ChildJobEvent
}

func RunChildJobLoopOnce(input ChildJobLoopInput) ChildJobLoopResult {
	jobs := cloneChildJobs(input.Jobs)
	reviewer := input.Reviewer
	if reviewer == nil {
		reviewer = DeterministicChildJobReviewer{}
	}
	result := ChildJobLoopResult{
		Jobs:            jobs,
		ObjectiveLedger: cloneStructuredObjectiveLedger(input.ObjectiveLedger),
	}
	activeIndex := activeChildJobIndex(jobs)
	if activeIndex >= 0 && !childJobTerminal(jobs[activeIndex]) {
		result.PlannerBlocked = true
		if input.ProposedParentJob != nil {
			result.ParentJobDeferred = true
			result.Events = append(result.Events, childJobEvent("child_job_parent_selection_deferred", "Parent planner selection deferred while active child job is non-terminal", map[string]string{
				"active_child_job": jobs[activeIndex].ID,
				"proposed_job":     input.ProposedParentJob.ID,
			}))
		}
	} else {
		activeIndex = firstNonTerminalChildJobIndex(jobs)
		if activeIndex < 0 && input.ProposedParentJob != nil {
			jobs = append(jobs, normalizeChildJob(*input.ProposedParentJob, ChildJobStatusPending))
			activeIndex = len(jobs) - 1
			result.Events = append(result.Events, childJobEvent("next_child_job_selected", "Parent planner selected next child job", map[string]string{
				"child_job_id": jobs[activeIndex].ID,
			}))
		}
	}
	if activeIndex < 0 {
		result.Jobs = jobs
		return result
	}

	job := normalizeChildJob(jobs[activeIndex], ChildJobStatusActive)
	if job.Status == ChildJobStatusPending {
		job.Status = ChildJobStatusActive
		result.Events = append(result.Events, childJobEvent("child_job_started", "Child job loop activated focused job", map[string]string{"child_job_id": job.ID}))
	}
	passed, missing := childJobEvidenceStatus(job, input.Observations, input.WorkingDirectory)
	result.Events = append(result.Events, childJobEvent("child_job_evidence_gathered", "Child job gathered deterministic evidence", map[string]string{
		"child_job_id":       job.ID,
		"passed_predicates":  strings.Join(passed, ","),
		"missing_predicates": strings.Join(missing, ","),
	}))
	if len(missing) == 0 {
		job.Status = ChildJobStatusVerifying
		review := reviewer.ReviewChildJob(ChildJobReviewInput{
			Job:                  job,
			WorkingDirectory:     input.WorkingDirectory,
			Observations:         input.Observations,
			PassedPredicates:     passed,
			MissingPredicates:    missing,
			DeterministicClosure: true,
		})
		job.ReviewerResult = review
		if review.Accepted {
			job.Status = ChildJobStatusComplete
			job.TerminalReason = firstNonEmpty(review.Reason, "child job accepted by scoped reviewer")
			jobs[activeIndex] = job
			result.Events = append(result.Events,
				childJobEvent("child_job_reviewer_accepted", "Scoped child reviewer accepted completion", map[string]string{"child_job_id": job.ID, "reason": review.Reason}),
				childJobEvent("child_job_completed", "Child job completed and left active queue", map[string]string{"child_job_id": job.ID, "terminal_reason": job.TerminalReason}),
				childJobEvent("workspace_route_refreshed_after_child_completion", "Workspace route and objective state refreshed after child completion", map[string]string{"child_job_id": job.ID}),
			)
			if childJobParentComplete(jobs, job) {
				result.ObjectiveLedger = reconcileObjectiveLedgerFromCompletedChildJob(result.ObjectiveLedger, job)
			}
		} else {
			job.Status = ChildJobStatusRepairing
			job.LatestFailurePacket = failurePacketForChildReview(job, review, passed, missing)
			result.NextAction = focusedChildJobAction(job, missing)
			result.Events = append(result.Events, childJobEvent("child_job_reviewer_rejected", "Scoped child reviewer rejected child completion", map[string]string{"child_job_id": job.ID, "reason": review.Reason}))
		}
	} else {
		job.Status = ChildJobStatusRepairing
		job.LatestFailurePacket = failurePacketForMissingEvidence(job, missing)
		result.NextAction = focusedChildJobAction(job, missing)
		result.Events = append(result.Events, childJobEvent("child_job_next_action_derived", "Child job derived focused next action from missing evidence", map[string]string{
			"child_job_id":       job.ID,
			"missing_predicates": strings.Join(missing, ","),
			"action_kind":        result.NextAction.Kind,
		}))
	}
	jobs[activeIndex] = job
	result.Jobs = jobs
	result.ActiveJob = &result.Jobs[activeIndex]
	return result
}

func childJobEvidenceStatus(job ChildJob, observations []StructuredCommandObservation, workingDir string) ([]string, []string) {
	passed := []string{}
	missing := []string{}
	if len(cleanStringList(job.RequiredEvidencePredicates)) == 0 && len(cleanStringList(job.ProofCommands)) == 0 {
		return passed, []string{"child_job_required_evidence_missing"}
	}
	for _, predicate := range job.RequiredEvidencePredicates {
		predicate = strings.TrimSpace(predicate)
		if predicate == "" {
			continue
		}
		if structuredEvidencePredicateSatisfied(predicate, observations, workingDir) {
			passed = append(passed, predicate)
		} else {
			missing = append(missing, predicate)
		}
	}
	for _, command := range job.ProofCommands {
		predicate := "command_passed:" + strings.TrimSpace(command)
		if structuredEvidencePredicateSatisfied(predicate, observations, workingDir) {
			passed = append(passed, predicate)
		} else {
			missing = append(missing, predicate)
		}
	}
	return passed, missing
}

func focusedChildJobAction(job ChildJob, missing []string) *ChildJobAction {
	action := &ChildJobAction{
		Kind:      "repair",
		JobID:     job.ID,
		Reason:    "missing child job evidence: " + strings.Join(missing, ","),
		Scope:     append([]string{}, job.ScopeFiles...),
		Evidence:  append([]string{}, missing...),
		Forbidden: forbiddenRepeatActionsForChildJob(job),
	}
	for _, predicate := range missing {
		switch {
		case strings.HasPrefix(predicate, "command_passed:"):
			action.Kind = "run_verification"
			action.Command = strings.TrimPrefix(predicate, "command_passed:")
			return action
		case strings.HasPrefix(predicate, "file_exists:"), strings.HasPrefix(predicate, "file_nonempty:"), strings.HasPrefix(predicate, "file_contains:"), strings.HasPrefix(predicate, "file_absent:"), strings.HasPrefix(predicate, "no_js_files_with_jsx:"):
			action.Kind = "patch"
		}
	}
	return action
}

func failurePacketForMissingEvidence(job ChildJob, missing []string) *FailurePacket {
	return &FailurePacket{
		ChildJobID:             job.ID,
		ObjectiveID:            job.ParentObjectiveID,
		FailureKind:            "missing_evidence",
		FailureFingerprint:     failureFingerprint(job.ID, missing),
		RequiredEvidence:       append([]string{}, job.RequiredEvidencePredicates...),
		MissingEvidence:        append([]string{}, missing...),
		ForbiddenNextActions:   forbiddenRepeatActionsForChildJob(job),
		RequiredNextBehavior:   "produce evidence for this child job only before selecting unrelated work",
		AllowedNextActionKinds: []string{"inspect", "patch", "run_verification"},
	}
}

func failurePacketForChildReview(job ChildJob, review ChildJobReviewerResult, passed, missing []string) *FailurePacket {
	packet := failurePacketForMissingEvidence(job, missing)
	packet.FailureKind = "reviewer_rejected"
	packet.StderrExcerpt = truncateStructuredObservation(review.Reason)
	packet.KnownState = "passed_predicates=" + strings.Join(passed, ",")
	return packet
}

func forbiddenRepeatActionsForChildJob(job ChildJob) []string {
	out := []string{}
	for _, attempt := range job.AttemptLedger {
		if attempt.ForbidRepeat && strings.TrimSpace(attempt.CommandOrPatch) != "" {
			out = append(out, attempt.CommandOrPatch)
		}
	}
	return cleanStringList(out)
}

func AppendChildJobAttempt(job ChildJob, obs StructuredCommandObservation, actor, role, model string) ChildJob {
	return AppendChildJobAttemptWithContext(job, obs, actor, role, model, "")
}

func AppendChildJobAttemptWithContext(job ChildJob, obs StructuredCommandObservation, actor, role, model, workingDir string) ChildJob {
	commandOrPatch := strings.TrimSpace(firstNonEmpty(obs.Command, obs.RejectedCommand))
	attempt := ChildJobAttempt{
		AttemptID:      firstNonEmpty(obs.CommandID, fmt.Sprintf("%s_attempt_%d", job.ID, len(job.AttemptLedger)+1)),
		Actor:          actor,
		Role:           role,
		Model:          model,
		ActionKind:     childJobActionKindFromObservation(obs),
		CommandOrPatch: commandOrPatch,
		Result:         "succeeded",
		CreatedAt:      time.Now().UTC(),
	}
	if obs.ExitCode != 0 || strings.TrimSpace(obs.RejectedCommand) != "" {
		attempt.Result = childJobAttemptResult(obs)
		attempt.FailureKind = classifyChildJobFailureKind(obs)
		attempt.ValidatorReason = childJobFailureExcerpt(firstNonEmpty(obs.Stderr, obs.Stdout))
		attempt.FullOutputRef = childJobFullOutputRef(job, obs)
		attempt.ForbidRepeat = commandOrPatch != ""
		job.LatestFailurePacket = failurePacketFromObservation(job, obs, attempt.FailureKind, workingDir)
	}
	job.AttemptLedger = append(job.AttemptLedger, attempt)
	return job
}

func failurePacketFromObservation(job ChildJob, obs StructuredCommandObservation, failureKind, workingDir string) *FailurePacket {
	command := firstNonEmpty(obs.Command, obs.RejectedCommand)
	_, missing := childJobEvidenceStatus(job, []StructuredCommandObservation{obs}, workingDir)
	return &FailurePacket{
		ChildJobID:              job.ID,
		ObjectiveID:             job.ParentObjectiveID,
		FailedAction:            command,
		CommandID:               obs.CommandID,
		CommandOrPatchSummary:   childJobFailureExcerpt(command),
		CWD:                     obs.CWD,
		ExitCode:                obs.ExitCode,
		StdoutExcerpt:           childJobFailureExcerpt(obs.Stdout),
		StderrExcerpt:           childJobFailureExcerpt(obs.Stderr),
		FailureKind:             failureKind,
		FailureFingerprint:      failureFingerprint(command, []string{obs.Stderr, obs.Stdout}),
		KnownState:              childJobKnownState(job, obs),
		RequiredEvidence:        append([]string{}, job.RequiredEvidencePredicates...),
		MissingEvidence:         missing,
		RequiredNextBehavior:    "repair the active child job using this failure packet before unrelated work",
		ForbiddenNextActions:    []string{command},
		AllowedNextActionKinds:  []string{"inspect", "patch", "run_verification"},
		FullOutputReferenceHint: childJobFullOutputRef(job, obs),
	}
}

func classifyChildJobFailureKind(obs StructuredCommandObservation) string {
	text := strings.ToLower(obs.Stderr + "\n" + obs.Stdout + "\n" + obs.RejectedCommand + "\n" + obs.CapabilityMemory)
	switch {
	case strings.TrimSpace(obs.RejectedCommand) != "" && strings.Contains(text, "placeholder"):
		return "placeholder_path_rejection"
	case strings.TrimSpace(obs.RejectedCommand) != "" && strings.Contains(text, "already completed"):
		return "repeated_command_rejection"
	case strings.TrimSpace(obs.RejectedCommand) != "":
		return "validator_rejection"
	case strings.Contains(text, "anti_loop: planner returned done=true") || strings.Contains(text, "done=true rejected") || strings.Contains(text, "false done"):
		return "false_done_rejection"
	case strings.Contains(text, "partial_failure"):
		return "partial_failure"
	case strings.Contains(text, "toolchain_feedback") || strings.Contains(text, "compile") || strings.Contains(text, "syntax") || strings.Contains(text, "test failed") || strings.Contains(text, "failed tests"):
		return "toolchain_failure"
	case strings.Contains(text, "no such file or directory") || strings.Contains(text, "cannot stat"):
		return "missing_source_file"
	case strings.Contains(text, "artifact_validation_failed"):
		return "artifact_validation_failure"
	case strings.Contains(text, "placeholder"):
		return "validator_rejection"
	case obs.ExitCode != 0:
		return "command_failed"
	default:
		return "rejected"
	}
}

func childJobAttemptResult(obs StructuredCommandObservation) string {
	text := strings.ToLower(obs.Stderr + "\n" + obs.Stdout)
	switch {
	case strings.Contains(text, "partial_failure"):
		return "partial_failure"
	case strings.TrimSpace(obs.RejectedCommand) != "":
		return "rejected"
	case obs.ExitCode != 0:
		return "failed"
	default:
		return "succeeded"
	}
}

func childJobActionKindFromObservation(obs StructuredCommandObservation) string {
	command := strings.ToLower(strings.TrimSpace(firstNonEmpty(obs.Command, obs.RejectedCommand)))
	switch {
	case strings.TrimSpace(obs.RejectedCommand) != "":
		return "rejected_command"
	case strings.HasPrefix(command, "architect.apply") || strings.Contains(command, " > ") || strings.Contains(command, "tee ") || strings.Contains(command, "mv ") || strings.HasPrefix(command, "mv "):
		return "patch"
	case structuredCommandLooksVerifier(command):
		return "run_verification"
	case structuredCommandLooksReadOnlyEvidence(command):
		return "inspect"
	default:
		return "command"
	}
}

func childJobFailureExcerpt(text string) string {
	text = strings.TrimSpace(text)
	const max = 420
	if len(text) <= max {
		return text
	}
	return text[:max] + "\n...[truncated]"
}

func childJobFullOutputRef(job ChildJob, obs StructuredCommandObservation) string {
	output := strings.TrimSpace(obs.Stdout + "\n" + obs.Stderr)
	if len(output) <= 420 {
		return ""
	}
	return firstNonEmpty(obs.CommandID, fmt.Sprintf("%s_attempt_%d_output", job.ID, len(job.AttemptLedger)+1))
}

func childJobKnownState(job ChildJob, obs StructuredCommandObservation) string {
	parts := []string{}
	if obs.CommandID != "" {
		parts = append(parts, "command_id="+obs.CommandID)
	}
	if obs.CWD != "" {
		parts = append(parts, "cwd="+obs.CWD)
	}
	if len(job.ScopeFiles) > 0 {
		parts = append(parts, "scope="+strings.Join(job.ScopeFiles, ","))
	}
	return strings.Join(parts, "; ")
}

func ChildJobShouldRejectRepeat(job ChildJob, commandOrPatch string) bool {
	normalized := normalizeStructuredCommandForComparison(commandOrPatch)
	if normalized == "" {
		return false
	}
	for _, attempt := range job.AttemptLedger {
		if !attempt.ForbidRepeat {
			continue
		}
		if normalizeStructuredCommandForComparison(attempt.CommandOrPatch) == normalized {
			return true
		}
	}
	return false
}

func activeChildJobIndex(jobs []ChildJob) int {
	for i, job := range jobs {
		if childJobActive(job) {
			return i
		}
	}
	return -1
}

func firstNonTerminalChildJobIndex(jobs []ChildJob) int {
	for i, job := range jobs {
		if !childJobTerminal(job) {
			return i
		}
	}
	return -1
}

func childJobActive(job ChildJob) bool {
	switch job.Status {
	case ChildJobStatusActive, ChildJobStatusRepairing, ChildJobStatusVerifying:
		return true
	default:
		return false
	}
}

func childJobTerminal(job ChildJob) bool {
	switch job.Status {
	case ChildJobStatusComplete, ChildJobStatusFailedWithEvidence, ChildJobStatusSkippedWithEvidence, ChildJobStatusSuperseded:
		return true
	default:
		return false
	}
}

func normalizeChildJob(job ChildJob, defaultStatus ChildJobStatus) ChildJob {
	if strings.TrimSpace(job.ID) == "" {
		job.ID = "child_job"
	}
	if job.Status == "" {
		job.Status = defaultStatus
	}
	job.ScopeFiles = cleanStringList(job.ScopeFiles)
	job.RequiredEvidencePredicates = cleanStringList(job.RequiredEvidencePredicates)
	job.ProofCommands = cleanStringList(job.ProofCommands)
	return job
}

func reconcileObjectiveLedgerFromCompletedChildJob(ledger []StructuredObjective, job ChildJob) []StructuredObjective {
	if strings.TrimSpace(job.ParentObjectiveID) == "" {
		return ledger
	}
	out := cloneStructuredObjectiveLedger(ledger)
	for i := range out {
		if out[i].ID != job.ParentObjectiveID {
			continue
		}
		out[i].Status = "satisfied"
		out[i].Evidence = "child_job_complete:" + job.ID
	}
	return out
}

func childJobParentComplete(jobs []ChildJob, completed ChildJob) bool {
	parentID := strings.TrimSpace(completed.ParentObjectiveID)
	if parentID == "" {
		return false
	}
	sawSibling := false
	for _, job := range jobs {
		if strings.TrimSpace(job.ParentObjectiveID) != parentID {
			continue
		}
		sawSibling = true
		if job.ID == completed.ID {
			if completed.Status != ChildJobStatusComplete {
				return false
			}
			continue
		}
		if job.Status != ChildJobStatusComplete {
			return false
		}
	}
	return sawSibling
}

func cloneChildJobs(jobs []ChildJob) []ChildJob {
	out := make([]ChildJob, len(jobs))
	copy(out, jobs)
	for i := range out {
		out[i].ScopeFiles = append([]string{}, out[i].ScopeFiles...)
		out[i].RequiredEvidencePredicates = append([]string{}, out[i].RequiredEvidencePredicates...)
		out[i].ProofCommands = append([]string{}, out[i].ProofCommands...)
		out[i].AttemptLedger = append([]ChildJobAttempt{}, out[i].AttemptLedger...)
	}
	return out
}

func cloneStructuredObjectiveLedger(ledger []StructuredObjective) []StructuredObjective {
	out := make([]StructuredObjective, len(ledger))
	copy(out, ledger)
	return out
}

func failureFingerprint(seed string, parts []string) string {
	joined := strings.ToLower(strings.TrimSpace(seed + "|" + strings.Join(parts, "|")))
	if len(joined) > 160 {
		joined = joined[:160]
	}
	return joined
}

func childJobEvent(eventType, summary string, details map[string]string) ChildJobEvent {
	if details == nil {
		details = map[string]string{}
	}
	return ChildJobEvent{Type: eventType, Summary: summary, Details: details}
}

func ChildJobFromObjectiveWorkItem(item ObjectiveWorkItem) ChildJob {
	predicates := cleanStringList(item.EvidencePredicates)
	for _, evidence := range item.RequiredEvidence {
		switch evidence {
		case EvidenceKindCommand:
			predicates = append(predicates, "command_passed:"+firstNonEmpty(firstWorkItemCommand(item), ""))
		case EvidenceKindFileDiff:
			for _, path := range item.Scope.Paths {
				if strings.TrimSpace(path) != "" {
					predicates = append(predicates, "file_nonempty:"+path)
				}
			}
		case EvidenceKindRead:
			for _, path := range item.Scope.Paths {
				if strings.TrimSpace(path) != "" {
					predicates = append(predicates, "file_exists:"+path)
				}
			}
		case EvidenceKindDeleteSafety:
			for _, path := range item.Scope.Paths {
				if strings.TrimSpace(path) != "" {
					predicates = append(predicates, "file_absent:"+path)
				}
			}
		}
	}
	return ChildJob{
		ID:                         item.ID,
		ParentObjectiveID:          item.ParentID,
		Goal:                       item.Instruction,
		Status:                     childJobStatusFromWorkItemStatus(item.Status),
		ScopeFiles:                 append([]string{}, item.Scope.Paths...),
		RequiredEvidencePredicates: cleanStringList(predicates),
	}
}

func BuildChildJobsFromObjectiveWorkItems(items []ObjectiveWorkItem) []ChildJob {
	jobs := []ChildJob{}
	var walk func([]ObjectiveWorkItem)
	walk = func(items []ObjectiveWorkItem) {
		for _, item := range items {
			if len(item.Children) > 0 {
				walk(item.Children)
				continue
			}
			jobs = append(jobs, ChildJobFromObjectiveWorkItem(item))
		}
	}
	walk(items)
	return jobs
}

func firstWorkItemCommand(item ObjectiveWorkItem) string {
	for _, evidence := range item.EvidenceRefs {
		if strings.TrimSpace(evidence.Command) != "" {
			return evidence.Command
		}
	}
	return ""
}

func childJobStatusFromWorkItemStatus(status WorkItemStatus) ChildJobStatus {
	switch status {
	case WorkItemStatusPassed:
		return ChildJobStatusComplete
	case WorkItemStatusFailed:
		return ChildJobStatusFailedWithEvidence
	case WorkItemStatusBlocked:
		return ChildJobStatusFailedWithEvidence
	case WorkItemStatusRunning:
		return ChildJobStatusActive
	default:
		return ChildJobStatusPending
	}
}

func RouteFilesAfterChildCompletion(route TaskRoute, completed ChildJob) TaskRoute {
	if completed.Status != ChildJobStatusComplete {
		return route
	}
	remove := map[string]bool{}
	add := map[string]bool{}
	for _, predicate := range completed.RequiredEvidencePredicates {
		kind, rest, ok := strings.Cut(predicate, ":")
		if !ok {
			continue
		}
		path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(rest)))
		switch strings.TrimSpace(kind) {
		case "file_absent":
			remove[path] = true
		case "file_exists", "file_nonempty":
			add[path] = true
		}
	}
	next := route
	files := []string{}
	seen := map[string]bool{}
	for _, path := range route.LikelyFiles {
		clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
		if clean == "." || remove[clean] || seen[clean] {
			continue
		}
		seen[clean] = true
		files = append(files, path)
	}
	for path := range add {
		if path == "." || seen[path] {
			continue
		}
		seen[path] = true
		files = append(files, path)
	}
	next.LikelyFiles = files
	return next
}
