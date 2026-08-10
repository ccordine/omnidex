package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/specialist"
)

func (r *nativeRuntimeV3) runPlanning() error {
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	capability, err := r.readCapabilityAudit()
	if err != nil {
		return err
	}
	if plan, ok := buildV3CodingCoordinatorPlan(intent); ok {
		if err := validateV3Plan(plan, intent, capability); err != nil {
			return err
		}
		if err := r.validatePlannedSpecialists(plan); err != nil {
			return err
		}
		r.svc.emitStepEvent(r.claim.Authority, "plan_policy_selected", "strategy=direct_coding coordinator_steps=1 state=workspace")
		return r.persistV3Plan(plan)
	}
	workspaceArtifact, err := r.readWorkspaceArtifact()
	if err != nil {
		return err
	}
	retrievalArtifact, err := r.readRetrievalArtifact()
	if err != nil {
		return err
	}
	webArtifact, err := r.readWebArtifact()
	if err != nil {
		return err
	}
	projection := projectV3Memory(intent, retrievalArtifact, projectTag(r.claim.Job), sessionTagForJob(r.claim.Job), r.svc.retrievalLimit)
	payload := map[string]any{
		"intent":            intent,
		"capability_audit":  capability,
		"workspace":         workspaceArtifact,
		"memory_references": projection,
		"web_evidence":      webArtifact,
	}
	invocation, err := r.invocationFor(
		"executive_planner",
		"plan_authoritative_objectives",
		intent.UserGoal,
		100,
		intent.CompletionCriteria,
		[]string{artifactRef(artifacts.KindIntent, r.claim.Job.ID), artifactRef(artifacts.KindCapabilityAudit, r.claim.Job.ID), artifactRef(artifacts.KindWorkspace, r.claim.Job.ID), artifactRef(artifacts.KindRetrieval, r.claim.Job.ID), artifactRef(artifacts.KindWebEvidence, r.claim.Job.ID)},
		payload,
	)
	if err != nil {
		return err
	}
	modelName := r.svc.v3SpecialistModel(r.claim.Job, r.routing, "executive_planner", specialist.RolePlannerSpecialist, r.routing.Plan)
	validateOutput := func(output map[string]any) error {
		raw, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("marshal planner output: %w", err)
		}
		candidate, err := parseStrictV3Plan(string(raw))
		if err != nil {
			return err
		}
		if err := validateV3Plan(candidate, intent, capability); err != nil {
			return err
		}
		return r.validatePlannedSpecialists(candidate)
	}
	output, err := r.invokeSpecialist("v3_planning", "executive_planner", modelName, invocation, validateOutput)
	if err != nil {
		return err
	}
	rawPlannerOutput, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("marshal planner output: %w", err)
	}
	plan, err := parseStrictV3Plan(string(rawPlannerOutput))
	if err != nil {
		return err
	}
	if err := validateV3Plan(plan, intent, capability); err != nil {
		return err
	}
	if err := r.validatePlannedSpecialists(plan); err != nil {
		return err
	}
	return r.persistV3Plan(plan)
}

func (r *nativeRuntimeV3) runSubtask() error {
	assignment, err := parseV3SubtaskAssignment(r.contexts)
	if err != nil {
		return err
	}
	intent, err := r.readIntentArtifact()
	if err != nil {
		return err
	}
	audit, err := r.readCapabilityAudit()
	if err != nil {
		return err
	}
	authoritativeObjective, err := validateV3SubtaskAssignment(assignment, intent, audit)
	if err != nil {
		return err
	}
	var summary string
	var sources []string
	if requiresDirectCoding(authoritativeObjective) {
		summary, sources, err = r.runDirectCodingObjective(assignment, authoritativeObjective, intent.CompletionCriteria)
	} else {
		summary, sources, err = r.runSubtaskWithTools(assignment, authoritativeObjective)
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(summary) == "" {
		return fmt.Errorf("subtask %q returned an empty result", assignment.ID)
	}
	artifact := artifacts.SubtaskResultArtifact{
		SubtaskID:            assignment.ID,
		Kind:                 assignment.Kind,
		RoleID:               assignment.RoleID,
		ObjectiveID:          assignment.ObjectiveID,
		Objective:            strings.TrimSpace(authoritativeObjective.Description),
		Priority:             assignment.Priority,
		RequiredCapabilities: append([]string(nil), assignment.RequiredCapabilities...),
		Summary:              strings.TrimSpace(summary),
		Sources:              sources,
	}
	if err := validateV3SubtaskResult(artifact); err != nil {
		return err
	}
	if err := r.writeArtifact(artifacts.KindSubtaskResult, artifact); err != nil {
		return err
	}
	artifactJSON, err := json.Marshal(artifact)
	if err != nil {
		return fmt.Errorf("marshal subtask result %q: %w", assignment.ID, err)
	}
	return r.complete("subtask:"+assignment.ID, artifact.Summary, string(artifactJSON))
}
