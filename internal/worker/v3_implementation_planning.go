package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/specialist"
)

const maxImplementationManifestAttempts = 4

func (r *nativeRuntimeV3) createImplementationLedger(objective artifacts.Objective, constraints, criteria []string) (artifacts.ImplementationLedgerArtifact, error) {
	scope, err := r.svc.workspaceScopeForV3Job(r.claim.Job)
	if err != nil {
		return artifacts.ImplementationLedgerArtifact{}, err
	}
	workspaceContext, err := liveSubtaskWorkspaceContext(scope.Scanner, objective.Description)
	if err != nil {
		return artifacts.ImplementationLedgerArtifact{}, err
	}
	basePrompt, err := buildImplementationManifestPrompt(objective, constraints, criteria, workspaceContext)
	if err != nil {
		return artifacts.ImplementationLedgerArtifact{}, err
	}
	models := r.implementationManifestAttemptModels(r.implementationManifestModel(), maxImplementationManifestAttempts)
	var validationErr error
	rejected := ""
	for attempt := 1; attempt <= maxImplementationManifestAttempts; attempt++ {
		prompt := basePrompt
		mode := "initial"
		if attempt > 1 {
			mode = "repair"
			prompt, err = buildImplementationManifestRepairPrompt(objective, constraints, criteria, validationErr, rejected)
			if err != nil {
				return artifacts.ImplementationLedgerArtifact{}, err
			}
		}
		modelName := models[attempt-1]
		scopeName := fmt.Sprintf("v3_implementation_manifest_%s_%d", mode, attempt)
		r.svc.emitStepEvent(r.claim.Step.ID, "implementation_manifest_planning", fmt.Sprintf("attempt=%d/%d mode=%s model=%s", attempt, maxImplementationManifestAttempts, mode, safeLine(modelName, "unknown")))
		raw, err := r.svc.llmGenerateWithTrace(r.ctx, r.claim.Step.ID, scopeName, modelName, prompt)
		if err != nil {
			return artifacts.ImplementationLedgerArtifact{}, err
		}
		ledger, err := parseImplementationManifest(raw, objective, constraints, criteria)
		if err == nil {
			if err := r.persistImplementationLedger(ledger); err != nil {
				return artifacts.ImplementationLedgerArtifact{}, err
			}
			r.svc.emitStepEvent(r.claim.Step.ID, "implementation_manifest_accepted", fmt.Sprintf("attempt=%d model=%s", attempt, safeLine(modelName, "unknown")))
			r.svc.emitStepEvent(r.claim.Step.ID, "implementation_ledger_created", fmt.Sprintf("revision=%d items=%d context_policy=minimal", ledger.Revision, len(ledger.Items)))
			return ledger, nil
		}
		validationErr = err
		rejected = raw
		r.svc.emitStepEvent(r.claim.Step.ID, "implementation_manifest_rejected", fmt.Sprintf("attempt=%d model=%s reason=%s", attempt, safeLine(modelName, "unknown"), safeLine(trimForBudget(err.Error(), 5000), "invalid manifest")))
	}
	return artifacts.ImplementationLedgerArtifact{}, fmt.Errorf("implementation planner failed its typed manifest contract after %d attempts: %s", maxImplementationManifestAttempts, strings.TrimSpace(validationErr.Error()))
}

func (r *nativeRuntimeV3) implementationManifestModel() string {
	if explicit := metadataModel(r.claim.Job, "model_plan", ""); explicit != "" {
		return explicit
	}
	if fast := strings.TrimSpace(r.svc.models.Fast); fast != "" {
		return fast
	}
	return r.svc.v3SpecialistModel(r.claim.Job, "executive_planner", specialist.RolePlannerSpecialist, r.svc.models.Plan)
}

func (r *nativeRuntimeV3) implementationManifestAttemptModels(original string, count int) []string {
	candidates := []string{
		strings.TrimSpace(original),
		strings.TrimSpace(r.svc.models.Plan),
		strings.TrimSpace(r.svc.models.Specialist[specialist.RolePlannerSpecialist]),
		strings.TrimSpace(r.svc.models.Reasoning),
		strings.TrimSpace(r.svc.models.Analyze),
		strings.TrimSpace(r.svc.models.Fast),
	}
	unique := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" || containsStringExact(unique, candidate) {
			continue
		}
		unique = append(unique, candidate)
	}
	if len(unique) == 0 || count <= 0 {
		return nil
	}
	models := make([]string, count)
	for index := range models {
		models[index] = unique[min(index, len(unique)-1)]
	}
	return models
}
