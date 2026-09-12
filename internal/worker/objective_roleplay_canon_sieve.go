package worker

import (
	"context"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func resolveRoleplayCanonCandidateQueue(
	ctx context.Context,
	adapter portableObjectiveRoleplayCanonStation,
	input assemblyline.RoleplayCanonExtractionInput,
) (assemblyline.RoleplayCanonExtractionDecision, int, error) {
	resolveModel := func() (string, error) {
		return objectiveRoleplaySemanticModel(adapter.runtime)
	}
	call := func(
		ctx context.Context,
		subject string,
		job assemblyline.PortableJob,
		decode roleplayCanonRawLeafDecoder,
	) (any, int, error) {
		return runObjectivePortableRawLeafStation(
			ctx, adapter.runtime, subject, job,
			station.RoleplayCanonExtraction, resolveModel,
			objectiveRawLeafDecoder[any](decode),
		)
	}
	return resolveRoleplayCanonCandidateQueueWithCall(ctx, input, call)
}

func resolveRoleplayCanonCandidateQueueWithCall(
	ctx context.Context,
	input assemblyline.RoleplayCanonExtractionInput,
	call roleplayCanonRawLeafCall,
) (assemblyline.RoleplayCanonExtractionDecision, int, error) {
	inventoryJob, err := assemblyline.NewRoleplayCanonFactInventoryJob(input)
	if err != nil {
		return assemblyline.RoleplayCanonExtractionDecision{}, 0, err
	}
	inventory, inventoryCalls, err := callRoleplayCanonRawLeaf(
		ctx, call, "roleplay_canon_fact_inventory", inventoryJob,
		func(raw string) (assemblyline.RoleplayCanonFactInventory, error) {
			return assemblyline.DecodeRoleplayCanonFactInventory(input, raw)
		},
	)
	totalCalls := inventoryCalls
	if err != nil {
		return assemblyline.RoleplayCanonExtractionDecision{}, totalCalls, err
	}

	accepted := make([]string, 0, len(inventory.Candidates))
	processed := make(map[string]struct{}, len(inventory.Candidates))
	for _, candidate := range inventory.Candidates {
		if _, duplicate := processed[candidate]; duplicate {
			continue
		}
		processed[candidate] = struct{}{}
		authorizationInput := assemblyline.RoleplayCanonFactCandidateAuthorizationInput{
			Authority: input,
			Candidate: candidate,
		}
		authorizationJob, err := assemblyline.NewRoleplayCanonFactCandidateAuthorizationJob(
			authorizationInput,
		)
		if err != nil {
			return assemblyline.RoleplayCanonExtractionDecision{}, totalCalls, err
		}
		authorization, leafCalls, err := callRoleplayCanonRawLeaf(
			ctx, call, "roleplay_canon_fact_candidate_authorization",
			authorizationJob,
			func(raw string) (assemblyline.RoleplayCanonFactCandidateAuthorization, error) {
				return assemblyline.DecodeRoleplayCanonFactCandidateAuthorization(
					authorizationInput, raw,
				)
			},
		)
		totalCalls += leafCalls
		if err != nil {
			return assemblyline.RoleplayCanonExtractionDecision{}, totalCalls, err
		}
		if authorization.Relation == assemblyline.RoleplayCanonFactNotEstablished {
			continue
		}

		duplicate := false
		for _, acceptedFact := range accepted {
			relationInput := assemblyline.RoleplayCanonFactCandidateRelationInput{
				Candidate: candidate, AcceptedFact: acceptedFact,
			}
			relationJob, err := assemblyline.NewRoleplayCanonFactCandidateRelationJob(relationInput)
			if err != nil {
				return assemblyline.RoleplayCanonExtractionDecision{}, totalCalls, err
			}
			relation, relationDispatches, err := callRoleplayCanonRawLeaf(
				ctx, call, "roleplay_canon_fact_candidate_relation",
				relationJob,
				func(raw string) (assemblyline.RoleplayCanonFactCandidateRelation, error) {
					return assemblyline.DecodeRoleplayCanonFactCandidateRelation(relationInput, raw)
				},
			)
			totalCalls += relationDispatches
			if err != nil {
				return assemblyline.RoleplayCanonExtractionDecision{}, totalCalls, err
			}
			if relation.Relation == assemblyline.RoleplayCanonFactsEquivalent {
				duplicate = true
				break
			}
		}
		if !duplicate {
			accepted = append(accepted, candidate)
		}
	}

	decision, err := assemblyline.AssembleRoleplayCanonExtractionDecision(input, accepted)
	return decision, totalCalls, err
}
