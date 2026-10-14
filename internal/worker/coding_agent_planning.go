package worker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

const codingAgentObjectiveRequirementID = "objective"

func (runtime *nativeRuntimeV3) runAgentPlanningAction(configuration codingagent.Configuration) error {
	if runtime.svc.agentProviders == nil {
		return fmt.Errorf("Agents mode has no registered planning environment")
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
	request, err := runtime.directCodingRequest()
	if err != nil {
		return err
	}
	intake, err := runtime.agentIntakeContext(request, scope.Root)
	if err != nil {
		return err
	}
	providers, _, err := runtime.svc.agentProviders()
	if err != nil {
		return err
	}
	authority := (&directCodingSession{request: request}).directCodingAuthority()
	controller := codingagent.Controller{
		JobID: runtime.claim.Job.ID, Generation: runtime.claim.Authority.Generation,
		Root: scope.Root, Configuration: configuration,
		Requirements:     []codingagent.Requirement{{ID: codingAgentObjectiveRequirementID, Outcome: authority}},
		Objective:        &codingagent.Objective{Instruction: request.Instruction, Clarifications: request.Feedback},
		ArtifactBindings: intake.ArtifactBindings, PlanningEnvironment: agentPlanningEnvironment(),
		Providers:   providers,
		Environment: &codingAgentEnvironment{runtime: runtime, root: scope.Root, identity: identity},
		Evidence:    runtime.svc.repo.CodingAgentEvidence(runtime.claim.Authority),
		Activity: func(activity model.AgentActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID, Attempt: runtime.claim.Authority.Attempt, Kind: "agent_activity", AgentActivity: &activity})
		},
		CallActivity: func(activity model.AgentCallActivity) {
			runtime.svc.emitRuntimeEvent(RuntimeEvent{JobID: runtime.claim.Authority.JobID, StepID: runtime.claim.Authority.StepID, Attempt: runtime.claim.Authority.Attempt, Kind: "agent_call_activity", AgentCallActivity: &activity})
		},
	}
	var workload codingagent.Workload
	stopHeartbeat := runtime.svc.startProgressHeartbeat(runtime.ctx, runtime.claim.Authority, "scoped-agent-planning")
	defer stopHeartbeat()
	err = runtime.runWithSessionMessages(agentContext, func(ctx context.Context) error {
		var proposeErr error
		workload, proposeErr = controller.Propose(ctx)
		return proposeErr
	})
	if err != nil {
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
	prior, err := runtime.svc.repo.PriorCodingPlanDecisions(runtime.ctx, runtime.claim.Job.ID, runtime.claim.Job.CurrentGeneration)
	if err != nil {
		return fmt.Errorf("load prior coding plan decisions: %w", err)
	}
	writes := make([]queue.CodingPlanLeafWrite, len(workload.Tasks))
	for index, task := range workload.Tasks {
		statement, err := codingagent.PlanTaskStatement(task)
		if err != nil {
			return err
		}
		id, err := model.NewCodingPlanLeafID()
		if err != nil {
			return err
		}
		decision := model.CodingPlanDecisionPending
		origin := runtime.claim.Job.CurrentGeneration
		if retained, exists := prior[statement]; exists {
			id, decision, origin = retained.LeafID, retained.Decision, retained.OriginGeneration
		}
		relation := assemblyline.ApplicationRequirementCandidateResultRelationResult{Schema: assemblyline.ApplicationRequirementCandidateResultRelationSchemaV1, Relation: assemblyline.ApplicationRequirementNoDerivedResult}
		writes[index] = queue.CodingPlanLeafWrite{Leaf: model.CodingPlanLeaf{ID: id, Statement: statement, Decision: decision}, DecisionOriginGeneration: origin, ResultRelation: &relation}
	}
	command := queue.StoreCodingPlanReviewCommand{Authority: runtime.claim.Authority, Leaves: writes, AgentWorkload: &workload, IntakeContext: intake}
	var plan model.CodingPlan
	if err := runtime.transitionAfterSessionMessages(runtime.ctx, func() error {
		var storeErr error
		plan, storeErr = runtime.svc.repo.StoreCodingPlanReview(runtime.ctx, command)
		return storeErr
	}); err != nil {
		return err
	}
	runtime.svc.emitStepEvent(runtime.claim.Authority, "coding_plan_review_ready", fmt.Sprintf("generation=%d revision=%d tasks=%d", plan.Generation, plan.Revision, len(workload.Tasks)))
	return errCodingPlanReviewPending
}

func (runtime *nativeRuntimeV3) agentIntakeContext(request directCodingRequest, root string) (*codingagent.IntakeContext, error) {
	authority := (&directCodingSession{request: request}).directCodingAuthority()
	provenance, err := objectiveInstructionPathProvenance(runtime.ctx, root, authority)
	if err != nil {
		return nil, err
	}
	redacted, identities, err := assemblyline.RedactArtifactIdentities(authority, provenance)
	if err != nil {
		return nil, err
	}
	bindings := make([]codingagent.ArtifactBinding, len(identities))
	for index, identity := range identities {
		bindings[index] = codingagent.ArtifactBinding{Token: identity.Token, Value: identity.Value}
	}
	intake := &codingagent.IntakeContext{RedactedRequest: redacted, ArtifactBindings: bindings}
	return intake, intake.Validate()
}

func agentPlanningEnvironment() *codingagent.PlanningEnvironment {
	environment := &codingagent.PlanningEnvironment{NewFilesAllowed: true, Executables: []string{}}
	for _, name := range []string{"go", "python3", "sh"} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != "" {
			environment.Executables = append(environment.Executables, name)
		}
	}
	return environment
}
