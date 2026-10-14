package worker

import (
	"errors"
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
	workload, err := runtime.svc.repo.CodingAgentFrozenWorkload(runtime.ctx, runtime.claim.Authority)
	if err != nil {
		return err
	}
	providers, verifier, err := runtime.svc.agentProviders()
	if err != nil {
		return err
	}
	request, err := runtime.directCodingRequest()
	if err != nil {
		return err
	}
	intake, err := runtime.svc.repo.CodingAgentIntakeContext(runtime.ctx, runtime.claim.Authority)
	if err != nil {
		return err
	}
	if intake == nil {
		return fmt.Errorf("native agent execution requires its approved intake context")
	}
	requirements := []codingagent.Requirement{{ID: codingAgentObjectiveRequirementID, Outcome: (&directCodingSession{request: request}).directCodingAuthority()}}
	controller := codingagent.Controller{JobID: runtime.claim.Job.ID, Generation: runtime.claim.Authority.Generation,
		Root: scope.Root, Configuration: configuration, Requirements: requirements, Providers: providers, Verifier: verifier,
		Objective:        &codingagent.Objective{Instruction: request.Instruction, Clarifications: request.Feedback},
		ArtifactBindings: intake.ArtifactBindings, ApprovedWorkload: workload,
		Environment: &codingAgentEnvironment{runtime: runtime, root: scope.Root, identity: identity},
		Evidence:    runtime.svc.repo.CodingAgentEvidence(runtime.claim.Authority),
		Activity: func(activity model.AgentActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{
				JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID,
				Attempt: runtime.claim.Authority.Attempt, Kind: "agent_activity", AgentActivity: &activity,
			})
		},
		CallActivity: func(activity model.AgentCallActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{
				JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID,
				Attempt: runtime.claim.Authority.Attempt, Kind: "agent_call_activity", AgentCallActivity: &activity,
			})
		}}
	stopHeartbeat := runtime.svc.startProgressHeartbeat(runtime.ctx, runtime.claim.Authority, "scoped-agent-work")
	defer stopHeartbeat()
	if err := runtime.runWithSessionMessages(agentContext, controller.Run); err != nil {
		var questions *codingagent.QuestionsPending
		if errors.As(err, &questions) {
			if pauseErr := runtime.transitionAfterSessionMessages(agentContext, func() error {
				return runtime.svc.repo.PauseCodingAgentStep(agentContext, runtime.claim.Authority, questions.Questions)
			}); pauseErr != nil {
				return pauseErr
			}
		}
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
	entries = append([]codingagent.Entry{{Generation: runtime.claim.Authority.Generation, Phase: "plan_frozen", Workload: workload}}, entries...)
	recap, err := codingagent.CompletionRecap(entries, model.MaxChannelContentBytes)
	if err != nil {
		return err
	}
	return runtime.completeAppliedWorkspace(recap)
}
