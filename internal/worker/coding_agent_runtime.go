package worker

import (
	"fmt"
	"os"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
)

func (runtime *nativeRuntimeV3) runAgentCodingAction(configuration codingagent.Configuration) error {
	if runtime.svc.agentProviders == nil {
		return fmt.Errorf("Agents mode has no registered execution environment")
	}
	scope, err := runtime.svc.workspaceScopeForV3Job(runtime.ctx, runtime.claim.Job)
	if err != nil {
		return err
	}
	if err := runtime.acquireWorkspaceMutationFence(scope.Root); err != nil {
		return err
	}
	identity, err := projectroot.DirectoryIdentity(scope.Root)
	if err != nil {
		return err
	}
	if err := runtime.svc.requireNativeAgentWorkspace(runtime.claim.Job, scope, identity); err != nil {
		return err
	}
	if err := runtime.workspaceFence.Reattest(scope.Root); err != nil {
		return fmt.Errorf("native agent environment must own the exact selected workspace: %w", err)
	}
	plan, err := runtime.svc.repo.LoadFrozenCodingPlan(runtime.ctx, runtime.claim.Authority)
	if err != nil {
		return err
	}
	requirements := make([]codingagent.Requirement, len(plan.Leaves))
	for index, leaf := range plan.Leaves {
		requirements[index] = codingagent.Requirement{ID: string(leaf.Leaf.ID), Outcome: leaf.Leaf.Statement}
	}
	providers, verifier, err := runtime.svc.agentProviders()
	if err != nil {
		return err
	}
	request, err := runtime.directCodingRequest()
	if err != nil {
		return err
	}
	bindings, facts, planningEnvironment, err := runtime.agentPlanningContext(request, scope.Root)
	if err != nil {
		return err
	}
	controller := codingagent.Controller{JobID: runtime.claim.Job.ID, Generation: runtime.claim.Authority.Generation,
		Root: scope.Root, Configuration: configuration, Requirements: requirements, Providers: providers, Verifier: verifier,
		Objective:        &codingagent.Objective{Instruction: request.Instruction, Clarifications: request.Feedback},
		ArtifactBindings: bindings, PlanningEnvironment: planningEnvironment,
		ConstructionFacts: facts,
		Environment:       &codingAgentEnvironment{runtime: runtime, root: scope.Root, identity: identity},
		Evidence:          runtime.svc.repo.CodingAgentEvidence(runtime.claim.Authority)}
	stopHeartbeat := runtime.svc.startProgressHeartbeat(runtime.ctx, runtime.claim.Authority, "scoped-agent-work")
	defer stopHeartbeat()
	if err := controller.Run(runtime.ctx); err != nil {
		return err
	}
	return runtime.completeAppliedWorkspace("Completed the frozen plan with scoped execution, independent review and verified checks.")
}

func (service *Service) requireNativeAgentWorkspace(job model.Job, scope v3WorkspaceScope, identity string) error {
	if err := service.requireHostWorkspaceRoot(scope.Root); err != nil {
		return err
	}
	if job.Pipeline != model.PipelineChat {
		return nil
	}
	clientID, directoryID, err := projectroot.ParseClientWorkspaceIdentity(scope.Identity)
	if err != nil {
		return err
	}
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	localID, err := projectroot.ReadClientInstallationIdentity(configRoot)
	if err != nil {
		return fmt.Errorf("native agent requires the existing local client installation: %w", err)
	}
	if clientID != localID || directoryID != identity {
		return fmt.Errorf("selected workspace belongs to another environment; use an agent registered in that environment")
	}
	return nil
}
