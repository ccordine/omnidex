package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func (r *nativeRuntimeV3) runObjectiveResolve() error {
	if r == nil || r.svc == nil || r.claim == nil {
		return fmt.Errorf("conversation objective runtime requires a claimed job")
	}
	repositoryStations, err := newPortableObjectiveRepositoryGroundingStation(r)
	if err != nil {
		return err
	}
	result, err := runObjectiveTurn(
		r.ctx,
		r.claim.Job,
		runtimeConversationCandidateProvider{runtime: r},
		portableObjectiveContextSieveStations{runtime: r},
		portableObjectiveKindStation{runtime: r},
		portableObjectiveConversationStation{runtime: r},
		repositoryStations,
		objectiveWorkflows{
			ResolveModelPathProvenance: func() (assemblyline.ArtifactIdentityProvenance, error) {
				provenance, err := r.deriveObjectiveInstructionProvenance()
				if err == nil {
					r.objectivePathProvenance = provenance
				}
				return provenance, err
			},
			WorkspaceReplanContext: func(ctx context.Context, job model.Job) (assemblyline.ObjectiveContext, error) {
				continuity, err := r.svc.repo.ObjectiveContinuityAuthorities(ctx, job)
				if err != nil {
					return assemblyline.ObjectiveContext{}, err
				}
				return continuity.ReplanContext(), nil
			},
			WorkspaceMutation:     r.runObjectiveWorkspaceMutation,
			DatabaseRead:          r.acquireObjectiveDatabaseEvidence,
			RoleplaySimulation:    r.projectObjectiveRoleplaySimulation,
			RoleplayCanon:         portableObjectiveRoleplayCanonStation{runtime: r},
			RoleplayCanonDelta:    r.svc.repo.FilterNewRoleplayCanonFacts,
			RoleplayOngoingAction: portableObjectiveRoleplayOngoingActionStation{runtime: r},
			RoleplayResearch:      r.acquireObjectiveRoleplayResearch,
		},
	)
	if err != nil {
		return err
	}
	if !result.Complete {
		return fmt.Errorf("conversation objective %q returned without code-owned completion", result.ObjectiveID)
	}
	if result.Kind == assemblyline.ObjectiveKindWorkspaceMutation {
		return r.complete("objective_result", result.Output, result.Output)
	}
	output, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil {
		return err
	}
	return r.completeWithEvidence(
		"objective_result", output, result.ObjectiveID, records, result.RoleplayResponses,
		result.RoleplayUserCanon, result.RoleplayUserOngoingAction,
	)
}

func (r *nativeRuntimeV3) deriveObjectiveInstructionProvenance() (
	assemblyline.ArtifactIdentityProvenance,
	error,
) {
	if r == nil || r.svc == nil || r.claim == nil {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"objective instruction provenance requires a claimed job",
		)
	}
	scope, err := r.svc.workspaceScopeForV3Job(r.claim.Job)
	if err != nil {
		return assemblyline.ArtifactIdentityProvenance{}, err
	}
	provenance, err := objectiveInstructionPathProvenance(
		r.ctx, scope.Root, r.claim.Job.Instruction,
	)
	if err != nil {
		return assemblyline.ArtifactIdentityProvenance{}, fmt.Errorf(
			"derive objective instruction artifact provenance: %w", err,
		)
	}
	return provenance, nil
}

func (r *nativeRuntimeV3) runObjectiveWorkspaceMutation(
	_ context.Context,
	authority turnAuthority,
) (string, error) {
	if r.claim.Job.ID != authority.JobID || r.claim.Job.Instruction != authority.Instruction {
		return "", fmt.Errorf("workspace mutation authority does not match the claimed conversation job")
	}
	request, err := directCodingRequestFromObjectiveAuthority(authority)
	if err != nil {
		return "", err
	}
	return r.runDirectCodingSession(request)
}

func directCodingRequestFromObjectiveAuthority(
	authority turnAuthority,
) (directCodingRequest, error) {
	if err := authority.Context.Validate(); err != nil {
		return directCodingRequest{}, fmt.Errorf("workspace mutation objective context: %w", err)
	}
	replan := authority.Context.ReplanAuthority
	for _, contextCapsule := range authority.Context.Capsules {
		for _, source := range contextCapsule.Sources {
			if source.Namespace != "objective_replan" {
				return directCodingRequest{}, fmt.Errorf(
					"workspace mutation requires exact current-turn authority; contextual source %q has no registered referent resolution",
					source.Namespace,
				)
			}
			if replan == nil || source.ContentSHA256 != replan.FeedbackSHA256 {
				return directCodingRequest{}, fmt.Errorf(
					"workspace mutation replan context is not bound to exact same-job feedback authority",
				)
			}
		}
	}
	request := directCodingRequest{Instruction: authority.Instruction}
	if replan != nil {
		if replan.JobID != authority.JobID {
			return directCodingRequest{}, fmt.Errorf(
				"workspace mutation replan authority belongs to job %d, expected %d",
				replan.JobID, authority.JobID,
			)
		}
		request.Feedback = []string{replan.Feedback}
	}
	return request, nil
}
