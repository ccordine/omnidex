package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/webresearch"
)

func (r *nativeRuntimeV3) runObjectiveResolve() error {
	if r == nil || r.svc == nil || r.claim == nil {
		return fmt.Errorf("conversation objective runtime requires a claimed job")
	}
	modelProvenance, err := r.captureObjectiveInstructionProvenance()
	if err != nil {
		return err
	}
	r.objectivePathProvenance = modelProvenance
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
			ModelPathProvenance:   modelProvenance,
			WorkspaceMutation:     r.runObjectiveWorkspaceMutation,
			RepositoryRead:        r.acquireObjectiveRepositoryEvidence,
			ExternalAnswer:        r.acquireObjectiveExternalEvidence,
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
	output, records, err := prepareObjectiveTurnCompletion(result)
	if err != nil {
		return err
	}
	return r.completeWithEvidence(
		"objective_result", output, result.ObjectiveID, records, result.RoleplayResponses,
		result.RoleplayUserCanon, result.RoleplayUserOngoingAction,
	)
}

func (r *nativeRuntimeV3) captureObjectiveInstructionProvenance() (
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
	indexed, err := r.captureExistingRepositoryIndex(scope.Root)
	if err != nil {
		return assemblyline.ArtifactIdentityProvenance{}, err
	}
	paths := make([]string, len(indexed.Snapshot.Files))
	for index, file := range indexed.Snapshot.Files {
		paths[index] = file.Path
	}
	provenance, err := modelcontext.NewArtifactIdentityProvenance(paths)
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

func (r *nativeRuntimeV3) acquireObjectiveExternalEvidence(
	ctx context.Context,
	authority turnAuthority,
) (objectiveExternalAnswer, error) {
	if ctx == nil || r == nil || r.svc == nil || r.claim == nil || r.svc.webSearch == nil {
		return objectiveExternalAnswer{}, fmt.Errorf("external-answer objective requires configured web acquisition")
	}
	if authority.JobID != r.claim.Job.ID || authority.Instruction != r.claim.Job.Instruction {
		return objectiveExternalAnswer{}, fmt.Errorf("external-answer authority does not match the claimed job")
	}
	stations, err := newRoutedWebStations(func(id station.ID) webresearch.PortableRuntime {
		return runtimeWebPortableRuntime(r, id)
	})
	if err != nil {
		return objectiveExternalAnswer{}, err
	}
	query := authority.Instruction
	if err := validateObjectiveModelInput(
		authority, "external-answer model question", authority.ModelInstruction,
	); err != nil {
		return objectiveExternalAnswer{}, err
	}
	machine, err := webresearch.New(webresearch.Objective{
		ID:                 webresearch.ObjectiveID(objectiveTurnID(authority, assemblyline.ObjectiveKindExternalAnswer)),
		Question:           authority.ModelInstruction,
		Context:            assemblyline.CloneObjectiveContext(authority.Context),
		InitialQuery:       query,
		KnownArtifactPaths: append([]string{}, authority.ModelArtifactPaths...),
		Status:             webresearch.ObjectivePending,
	}, objectiveWebResearchConfig(), r.svc.webSearch, stations.relevance,
		stations.synthesis)
	if err != nil {
		return objectiveExternalAnswer{}, err
	}
	result, err := machine.Run(ctx)
	if err != nil {
		return objectiveExternalAnswer{}, err
	}
	return objectiveExternalAnswerFromWeb(result)
}
