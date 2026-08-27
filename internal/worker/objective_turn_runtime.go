package worker

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/webresearch"
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

func (r *nativeRuntimeV3) runObjectiveWorkspaceMutation(
	_ context.Context,
	authority turnAuthority,
) (string, error) {
	if r.claim.Job.ID != authority.JobID || r.claim.Job.Instruction != authority.Instruction {
		return "", fmt.Errorf("workspace mutation authority does not match the claimed conversation job")
	}
	request := directCodingRequest{Instruction: authority.Instruction}
	for _, capsule := range authority.Context.Capsules {
		isReplan := false
		for _, source := range capsule.Sources {
			isReplan = isReplan || source.Namespace == "objective_replan"
		}
		if isReplan {
			request.Feedback = append(request.Feedback, capsule.Content)
		} else {
			request.AdditionalAuthority = append(request.AdditionalAuthority, capsule.Content)
		}
	}
	if request.Instruction != authority.Instruction {
		return "", fmt.Errorf("direct coding request rewrote exact conversation authority")
	}
	return r.runDirectCodingSession(request)
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
	if err := requireIndependentWebReviewRoutes(r.routing); err != nil {
		return objectiveExternalAnswer{}, err
	}
	identityGuard := &webModelIdentityGuard{}
	stations, err := newRoutedWebStations(func(id station.ID) webresearch.PortableRuntime {
		return runtimeWebPortableRuntime(r, id, identityGuard)
	})
	if err != nil {
		return objectiveExternalAnswer{}, err
	}
	query := strings.TrimSpace(authority.Instruction)
	if len(query) > 1_024 {
		query = ""
	}
	machine, err := webresearch.New(webresearch.Objective{
		ID:           webresearch.ObjectiveID(objectiveTurnID(authority, assemblyline.ObjectiveKindExternalAnswer)),
		Question:     authority.Instruction,
		Context:      assemblyline.CloneObjectiveContext(authority.Context),
		InitialQuery: query,
		Acceptance: []webresearch.AcceptancePredicate{
			webresearch.AcceptanceGroundedSynthesis,
			webresearch.AcceptanceExactCitations,
			webresearch.AcceptanceClaimEvidenceReview,
		},
		Status: webresearch.ObjectivePending,
	}, objectiveWebResearchConfig(), r.svc.webSearch, stations.terms, stations.relevance,
		stations.synthesis, stations.correction, stations.review)
	if err != nil {
		return objectiveExternalAnswer{}, err
	}
	result, err := machine.Run(ctx)
	if err != nil {
		return objectiveExternalAnswer{}, err
	}
	return objectiveExternalAnswerFromWeb(result)
}
