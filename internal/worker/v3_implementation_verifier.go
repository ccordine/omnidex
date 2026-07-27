package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/specialist"
	"github.com/gryph/omnidex/internal/specialists"
	toolruntime "github.com/gryph/omnidex/internal/tools"
)

const maxImplementationTriageContractAttempts = 2

func (r *nativeRuntimeV3) runImplementationVerificationItem(ledger *artifacts.ImplementationLedgerArtifact, index int, spec specialists.Spec) (*subtaskToolRecord, error) {
	item := &ledger.Items[index]
	if item.Kind != artifacts.ImplementationWorkKindVerification || item.Command == nil {
		return nil, fmt.Errorf("work item %q is not complete verification work", item.ID)
	}
	if item.Attempts >= maxImplementationVerifyRuns {
		return nil, r.failImplementationItem(ledger, index, fmt.Sprintf("exhausted the %d-run verification budget", maxImplementationVerifyRuns))
	}
	item.Status = artifacts.ImplementationWorkStatusRunning
	item.Attempts++
	ledger.Revision++
	if err := r.persistImplementationLedger(*ledger); err != nil {
		return nil, err
	}
	commandText := strings.Join(append([]string{item.Command.Program}, item.Command.Args...), " ")
	r.svc.emitStepEvent(r.claim.Step.ID, "implementation_verification_started", fmt.Sprintf("id=%s attempt=%d/%d command=%s", item.ID, item.Attempts, maxImplementationVerifyRuns, safeLine(commandText, "unknown")))

	input := map[string]any{"program": item.Command.Program, "args": append([]string(nil), item.Command.Args...)}
	if item.Command.TimeoutSeconds > 0 {
		input["timeout_seconds"] = item.Command.TimeoutSeconds
	}
	record, callErr := r.executeSubtaskToolCall(spec, ledger.Objective, []string{"workspace.write", "command.run"}, toolruntime.Call{Name: "command.run", Input: input})
	if callErr != nil {
		if toolruntime.IsCallRejected(callErr) {
			return &record, r.failImplementationItem(ledger, index, "authoritative verification command was rejected: "+callErr.Error())
		}
		return &record, callErr
	}
	if toolResultSucceeded(record.Result) {
		item.Status = artifacts.ImplementationWorkStatusCompleted
		item.LastError = ""
		item.ResultSummary = commandText + " passed: " + record.Result.Summary
		ledger.Revision++
		if err := r.persistImplementationLedger(*ledger); err != nil {
			return &record, err
		}
		r.svc.emitStepEvent(r.claim.Step.ID, "implementation_verification_completed", fmt.Sprintf("id=%s command=%s", item.ID, safeLine(commandText, "unknown")))
		return &record, nil
	}

	failure := implementationVerificationFailure(record)
	if item.Attempts >= maxImplementationVerifyRuns {
		return &record, r.failImplementationItem(ledger, index, failure)
	}
	triage, err := r.triageImplementationFailure(*ledger, *item, failure)
	if err != nil {
		return &record, err
	}
	feedback := strings.Join([]string{
		"AUTHORITATIVE VERIFICATION FAILED.",
		failure,
		"TRIAGE INSTRUCTION: " + strings.TrimSpace(triage.Feedback),
		"Correct this same file contract. Preserve completed unrelated work and do not weaken tests.",
	}, "\n")
	if err := reopenImplementationOwner(ledger, triage.OwnerID, feedback); err != nil {
		return &record, r.failImplementationItem(ledger, index, err.Error())
	}
	item.Status = artifacts.ImplementationWorkStatusPending
	item.LastError = trimForBudget(failure, 6000)
	item.ResultSummary = ""
	ledger.Revision++
	if err := r.persistImplementationLedger(*ledger); err != nil {
		return &record, err
	}
	r.svc.emitStepEvent(r.claim.Step.ID, "implementation_failure_routed", fmt.Sprintf("verification=%s owner=%s preserved_completed=true reason=%s", item.ID, triage.OwnerID, safeLine(triage.Feedback, "correction required")))
	return &record, nil
}

func (r *nativeRuntimeV3) triageImplementationFailure(ledger artifacts.ImplementationLedgerArtifact, verification artifacts.ImplementationWorkItem, failure string) (implementationTriageDecision, error) {
	if decision, found := deterministicImplementationFailureRoute(ledger, verification, failure); found {
		r.svc.emitStepEvent(r.claim.Step.ID, "implementation_triage_completed", fmt.Sprintf(
			"verification=%s owner=%s source=observed_file_path",
			safeLine(verification.ID, "unknown"), safeLine(decision.OwnerID, "unknown"),
		))
		return decision, nil
	}
	basePrompt, err := buildImplementationTriagePrompt(ledger, verification, failure)
	if err != nil {
		return implementationTriageDecision{}, err
	}
	modelName := r.svc.v3SpecialistModel(r.claim.Job, "analysis_specialist", specialist.RoleAnalysisSpecialist, r.svc.models.Analyze)
	validationFailure := ""
	for attempt := 1; attempt <= maxImplementationTriageContractAttempts; attempt++ {
		prompt := basePrompt
		if validationFailure != "" {
			prompt += "\n\nDIRECT_VALIDATION_FAILURE:\n" + validationFailure + "\nReturn a corrected triage envelope only."
		}
		raw, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, fmt.Sprintf("v3_failure_triage_%s_%d", verification.ID, attempt), modelName, prompt)
		if err != nil {
			return implementationTriageDecision{}, err
		}
		decision, err := parseImplementationTriageDecision(raw, ledger, verification)
		if err == nil {
			return decision, nil
		}
		validationFailure = trimForBudget(err.Error(), 3000)
		r.svc.emitStepEvent(r.claim.Step.ID, "implementation_triage_rejected", fmt.Sprintf("verification=%s attempt=%d reason=%s", verification.ID, attempt, safeLine(validationFailure, "invalid triage")))
	}
	return implementationTriageDecision{}, fmt.Errorf("failure triager failed its typed contract for verification %q after %d attempts: %s", verification.ID, maxImplementationTriageContractAttempts, validationFailure)
}

func implementationVerificationFailure(record subtaskToolRecord) string {
	lines := []string{"Command: " + subtaskCommandText(record.Call)}
	if exitCode, ok := record.Result.Output["exit_code"]; ok {
		lines = append(lines, fmt.Sprintf("Exit code: %v", exitCode))
	}
	if stderr := toolResultText(record.Result.Output, "stderr"); stderr != "" {
		lines = append(lines, "Stderr:\n"+trimForBudget(stderr, 5000))
	}
	if stdout := toolResultText(record.Result.Output, "stdout"); stdout != "" {
		lines = append(lines, "Stdout:\n"+trimForBudget(stdout, 5000))
	}
	if len(lines) == 1 {
		lines = append(lines, "The command returned an unsuccessful result without stdout or stderr.")
	}
	return strings.Join(lines, "\n")
}
