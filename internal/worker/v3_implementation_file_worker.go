package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/specialists"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

const maxImplementationReviewContractAttempts = 2

func (r *nativeRuntimeV3) runImplementationFileItem(ledger *artifacts.ImplementationLedgerArtifact, index int, spec specialists.Spec) (*subtaskToolRecord, error) {
	item := &ledger.Items[index]
	if item.Kind != artifacts.ImplementationWorkKindFile {
		return nil, fmt.Errorf("work item %q is not file work", item.ID)
	}
	if item.Attempts >= maxImplementationItemAttempts {
		return nil, r.failImplementationItem(ledger, index, fmt.Sprintf("exhausted the %d-attempt file budget", maxImplementationItemAttempts))
	}
	item.Status = artifacts.ImplementationWorkStatusRunning
	item.Attempts++
	ledger.Revision++
	if err := r.persistImplementationLedger(*ledger); err != nil {
		return nil, err
	}
	r.svc.emitStepEvent(r.claim.Step.ID, "implementation_work_started", fmt.Sprintf(
		"id=%s discipline=%s path=%s attempt=%d/%d",
		safeLine(item.ID, "unknown"), safeLine(item.Discipline, "unknown"), safeLine(item.Path, "unknown"), item.Attempts, maxImplementationItemAttempts,
	))

	scope, err := r.svc.workspaceScopeForV3Job(r.claim.Job)
	if err != nil {
		return nil, err
	}
	context, err := readImplementationWorkContext(scope.Root, *ledger, *item)
	if err != nil {
		return nil, r.retryImplementationItem(ledger, index, err.Error())
	}
	prompt, err := buildImplementationWriterPrompt(*item, context, item.LastError)
	if err != nil {
		return nil, err
	}
	modelName := r.implementationWriterAttemptModels(*item, maxImplementationItemAttempts)[item.Attempts-1]
	raw, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, fmt.Sprintf("v3_work_item_writer_%s_%d", item.ID, item.Attempts), modelName, prompt)
	if err != nil {
		return nil, err
	}
	decision, err := parseImplementationFileDecision(raw, *item)
	if err != nil {
		return nil, r.retryImplementationItem(ledger, index, "FILE WORKER RESPONSE REJECTED: "+err.Error())
	}
	candidateContent := decision.Content
	if decision.Status == "satisfied" {
		if !context.Target.Exists || context.Target.Content == "" {
			return nil, r.retryImplementationItem(ledger, index, "SATISFIED CLAIM REJECTED: the assigned target does not exist with non-empty content. Return a complete write.")
		}
		candidateContent = context.Target.Content
	}
	if err := validateImplementationCandidateContent(*item, candidateContent); err != nil {
		return nil, r.retryImplementationItem(ledger, index, "DETERMINISTIC SOURCE CHECK FAILED: "+err.Error())
	}
	if decision.Status == "satisfied" {
		if checkRecord, checkErr := r.checkWrittenImplementationFile(ledger, index, spec); checkErr != nil {
			return checkRecord, checkErr
		}
		review, err := r.reviewImplementationCandidate(*item, context, candidateContent)
		if err != nil {
			return nil, err
		}
		if review.Verdict == "revise" {
			feedback := "INDEPENDENT SEMANTIC REVIEW REJECTED THE COMPILED CANDIDATE:\n- " + strings.Join(review.Findings, "\n- ")
			return nil, r.retryImplementationItem(ledger, index, feedback)
		}
		digest := sha256.Sum256([]byte(candidateContent))
		item.Status = artifacts.ImplementationWorkStatusCompleted
		item.LastError = ""
		item.ContentSHA256 = hex.EncodeToString(digest[:])
		item.ResultSummary = implementationReviewSummary(review, true)
		ledger.Revision++
		if err := r.persistImplementationLedger(*ledger); err != nil {
			return nil, err
		}
		r.svc.emitStepEvent(r.claim.Step.ID, "implementation_work_completed", fmt.Sprintf("id=%s path=%s mutation=unchanged review=%s", item.ID, item.Path, review.Authority))
		return nil, nil
	}

	operation := "create"
	if context.Target.Exists {
		operation = "replace"
	}
	call := toolruntime.Call{Name: "workspace.write", Input: map[string]any{
		"path": item.Path, "operation": operation, "content": candidateContent,
	}}
	record, callErr := r.executeSubtaskToolCall(spec, ledger.Objective, []string{"workspace.write", "command.run"}, call)
	if callErr != nil {
		if toolruntime.IsCallRejected(callErr) {
			return &record, r.retryImplementationItem(ledger, index, "WORKSPACE WRITE REJECTED: "+callErr.Error())
		}
		return &record, callErr
	}
	hash, _ := record.Result.Output["content_sha256"].(string)
	if strings.TrimSpace(hash) == "" {
		return &record, fmt.Errorf("workspace.write for work item %q returned no content_sha256", item.ID)
	}
	if checkRecord, checkErr := r.checkWrittenImplementationFile(ledger, index, spec); checkErr != nil {
		if checkRecord != nil {
			return checkRecord, checkErr
		}
		return &record, checkErr
	}
	reviewContext, err := readImplementationWorkContext(scope.Root, *ledger, *item)
	if err != nil {
		return &record, r.retryImplementationItem(ledger, index, "POST-WRITE CONTEXT CHECK FAILED: "+err.Error())
	}
	review, err := r.reviewImplementationCandidate(*item, reviewContext, candidateContent)
	if err != nil {
		return &record, err
	}
	if review.Verdict == "revise" {
		feedback := "INDEPENDENT SEMANTIC REVIEW REJECTED THE COMPILED CANDIDATE:\n- " + strings.Join(review.Findings, "\n- ")
		return &record, r.retryImplementationItem(ledger, index, feedback)
	}
	item.Status = artifacts.ImplementationWorkStatusCompleted
	item.LastError = ""
	item.ContentSHA256 = hash
	item.ResultSummary = record.Result.Summary
	ledger.Revision++
	if err := r.persistImplementationLedger(*ledger); err != nil {
		return &record, err
	}
	r.svc.emitStepEvent(r.claim.Step.ID, "implementation_work_completed", fmt.Sprintf("id=%s path=%s mutation=%s review=%s", item.ID, item.Path, operation, review.Authority))
	return &record, nil
}

func (r *nativeRuntimeV3) implementationWriterModel(item artifacts.ImplementationWorkItem) string {
	if explicit := metadataModel(r.claim.Job, "model_execute", ""); explicit != "" {
		return explicit
	}
	if implementationWriterUsesFastModel(item) && strings.TrimSpace(r.svc.models.Fast) != "" {
		return strings.TrimSpace(r.svc.models.Fast)
	}
	return r.svc.v3SpecialistModel(r.claim.Job, "subtask_executor", specialist.RoleSubtaskExecutorSpecialist, r.svc.models.Analyze)
}

func (r *nativeRuntimeV3) implementationWriterAttemptModels(item artifacts.ImplementationWorkItem, count int) []string {
	if count <= 0 {
		return nil
	}
	if explicit := metadataModel(r.claim.Job, "model_execute", ""); explicit != "" {
		models := make([]string, count)
		for index := range models {
			models[index] = explicit
		}
		return models
	}
	executor := strings.TrimSpace(r.svc.models.Specialist[specialist.RoleSubtaskExecutorSpecialist])
	if executor == "" {
		executor = strings.TrimSpace(r.svc.v3SpecialistModel(
			r.claim.Job,
			"subtask_executor",
			specialist.RoleSubtaskExecutorSpecialist,
			r.svc.models.Analyze,
		))
	}
	if !implementationWriterUsesFastModel(item) {
		strongest := executor
		if strongest == "" {
			strongest = strings.TrimSpace(r.implementationWriterModel(item))
		}
		models := make([]string, count)
		for index := range models {
			models[index] = strongest
		}
		return models
	}
	candidates := []string{
		strings.TrimSpace(r.implementationWriterModel(item)),
		strings.TrimSpace(r.svc.models.Reasoning),
		executor,
	}
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || containsStringExact(unique, candidate) {
			continue
		}
		unique = append(unique, candidate)
	}
	models := make([]string, count)
	for index := range models {
		models[index] = unique[min(index, len(unique)-1)]
	}
	return models
}

func implementationWriterUsesFastModel(item artifacts.ImplementationWorkItem) bool {
	return item.Discipline == artifacts.ImplementationDisciplineBootstrap ||
		item.Discipline == artifacts.ImplementationDisciplineEntrypoint ||
		item.Discipline == artifacts.ImplementationDisciplineTest ||
		item.Discipline == artifacts.ImplementationDisciplineDocumentation
}

func implementationSemanticReviewRequired(item artifacts.ImplementationWorkItem) bool {
	return item.Discipline != artifacts.ImplementationDisciplineBootstrap
}

func implementationReviewSummary(review implementationReviewDecision, existing bool) string {
	if review.Authority == "deterministic_bootstrap" {
		return "Server-side go.mod validation confirmed the bootstrap contract."
	}
	if existing {
		return "Independent semantic reviewer confirmed the existing target satisfies its file contract."
	}
	return "Independent semantic reviewer confirmed the written target satisfies its file contract."
}

func (r *nativeRuntimeV3) reviewImplementationCandidate(item artifacts.ImplementationWorkItem, context implementationWorkContext, content string) (implementationReviewDecision, error) {
	if !implementationSemanticReviewRequired(item) {
		decision := implementationReviewDecision{
			RoleID: "file_reviewer", WorkItemID: item.ID, Verdict: "pass",
			Authority: "deterministic_bootstrap",
		}
		r.svc.emitStepEvent(r.claim.Step.ID, "implementation_work_reviewed", fmt.Sprintf(
			"id=%s verdict=pass findings=0 authority=%s",
			item.ID, decision.Authority,
		))
		return decision, nil
	}
	basePrompt, err := buildImplementationReviewPrompt(item, context, content)
	if err != nil {
		return implementationReviewDecision{}, err
	}
	modelName := r.svc.v3SpecialistModel(r.claim.Job, "verifier", specialist.RoleReviewVerificationSpecialist, r.svc.models.Analyze)
	validationFailure := ""
	for attempt := 1; attempt <= maxImplementationReviewContractAttempts; attempt++ {
		prompt := basePrompt
		if validationFailure != "" {
			prompt += "\n\nDIRECT_VALIDATION_FAILURE:\n" + validationFailure + "\nReturn a corrected review envelope only."
		}
		raw, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, fmt.Sprintf("v3_work_item_review_%s_%d", item.ID, attempt), modelName, prompt)
		if err != nil {
			return implementationReviewDecision{}, err
		}
		decision, err := parseImplementationReviewDecision(raw, item)
		if err == nil {
			decision.Authority = "semantic_llm"
			r.svc.emitStepEvent(r.claim.Step.ID, "implementation_work_reviewed", fmt.Sprintf("id=%s verdict=%s findings=%d authority=%s", item.ID, decision.Verdict, len(decision.Findings), decision.Authority))
			return decision, nil
		}
		validationFailure = trimForBudget(err.Error(), 3000)
	}
	return implementationReviewDecision{}, fmt.Errorf("file reviewer failed its typed contract for work item %q after %d attempts: %s", item.ID, maxImplementationReviewContractAttempts, validationFailure)
}

func (r *nativeRuntimeV3) retryImplementationItem(ledger *artifacts.ImplementationLedgerArtifact, index int, feedback string) error {
	item := &ledger.Items[index]
	feedback = trimForBudget(strings.TrimSpace(feedback), 6000)
	if item.Attempts >= maxImplementationItemAttempts {
		return r.failImplementationItem(ledger, index, feedback)
	}
	item.Status = artifacts.ImplementationWorkStatusPending
	item.LastError = feedback
	item.ResultSummary = ""
	ledger.Revision++
	if err := r.persistImplementationLedger(*ledger); err != nil {
		return err
	}
	r.svc.emitStepEvent(r.claim.Step.ID, "implementation_work_correction", fmt.Sprintf("id=%s next=same_owner reason=%s", item.ID, safeLine(feedback, "correction required")))
	return nil
}

func (r *nativeRuntimeV3) failImplementationItem(ledger *artifacts.ImplementationLedgerArtifact, index int, reason string) error {
	item := &ledger.Items[index]
	item.Status = artifacts.ImplementationWorkStatusFailed
	item.LastError = trimForBudget(strings.TrimSpace(reason), 6000)
	ledger.Revision++
	if err := r.persistImplementationLedger(*ledger); err != nil {
		return err
	}
	r.svc.emitStepEvent(r.claim.Step.ID, "implementation_work_failed", fmt.Sprintf("id=%s attempts=%d reason=%s", item.ID, item.Attempts, safeLine(item.LastError, "failed")))
	return fmt.Errorf("implementation work item %q failed after %d attempts: %s", item.ID, item.Attempts, item.LastError)
}
