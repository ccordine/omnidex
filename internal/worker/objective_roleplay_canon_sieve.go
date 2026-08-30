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
) (assemblyline.RoleplayCanonExtractionDecision, objectiveStationReceipt, error) {
	resolveModel := func() (string, error) {
		return objectiveRoleplaySemanticModel(adapter.runtime)
	}
	inventoryJob, err := assemblyline.NewRoleplayCanonFactInventoryJob(input)
	if err != nil {
		return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{}, err
	}
	inventory, receipt, err := runObjectiveReusablePortableRawLeafCall(
		ctx, adapter.runtime, "roleplay_canon_fact_inventory", inventoryJob,
		station.RoleplayCanonExtraction, resolveModel,
		func(raw string) (assemblyline.RoleplayCanonFactInventory, error) {
			return assemblyline.DecodeRoleplayCanonFactInventory(input, raw)
		},
		func(value assemblyline.RoleplayCanonFactInventory) error {
			return value.ValidateFor(input)
		},
	)
	totalCalls, allReused := receipt.Calls, receipt.Reused
	if err != nil {
		return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
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
			return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
		}
		authorization, leafReceipt, err := runObjectiveReusablePortableRawLeafCall(
			ctx, adapter.runtime, "roleplay_canon_fact_candidate_authorization",
			authorizationJob, station.RoleplayCanonExtraction, resolveModel,
			func(raw string) (assemblyline.RoleplayCanonFactCandidateAuthorization, error) {
				return assemblyline.DecodeRoleplayCanonFactCandidateAuthorization(
					authorizationInput, raw,
				)
			},
			func(value assemblyline.RoleplayCanonFactCandidateAuthorization) error {
				return value.ValidateFor(authorizationInput)
			},
		)
		totalCalls += leafReceipt.Calls
		allReused = allReused && leafReceipt.Reused
		if err != nil {
			return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
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
				return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
			}
			relation, relationReceipt, err := runObjectiveReusablePortableRawLeafCall(
				ctx, adapter.runtime, "roleplay_canon_fact_candidate_relation",
				relationJob, station.RoleplayCanonExtraction, resolveModel,
				func(raw string) (assemblyline.RoleplayCanonFactCandidateRelation, error) {
					return assemblyline.DecodeRoleplayCanonFactCandidateRelation(relationInput, raw)
				},
				func(value assemblyline.RoleplayCanonFactCandidateRelation) error {
					return value.ValidateFor(relationInput)
				},
			)
			totalCalls += relationReceipt.Calls
			allReused = allReused && relationReceipt.Reused
			if err != nil {
				return assemblyline.RoleplayCanonExtractionDecision{}, objectiveStationReceipt{Calls: totalCalls}, err
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
	return decision, objectiveStationReceipt{Calls: totalCalls, Reused: allReused}, err
}
