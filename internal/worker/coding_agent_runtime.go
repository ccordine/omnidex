package worker

import (
	"fmt"

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
	agentContext, cancelAgent, err := runtime.svc.nativeAgentWorkspaceContext(runtime.ctx, runtime.claim.Job, scope, identity)
	if err != nil {
		return err
	}
	defer cancelAgent()
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
	if err := controller.Run(agentContext); err != nil {
		return err
	}
	if err := agentContext.Err(); err != nil {
		return err
	}
	if err := runtime.workspaceFence.Reattest(scope.Root); err != nil {
		return err
	}
	entries, err := controller.Evidence.Read(agentContext)
	if err != nil {
		return err
	}
	recap, err := codingagent.CompletionRecap(entries, model.MaxChannelContentBytes)
	if err != nil {
		return err
	}
	return runtime.completeAppliedWorkspace(recap)
}
