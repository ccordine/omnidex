package worker

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// resolveDatabaseQueryPurposeQueue inventories one collection exactly once,
// then lets code own source order, exact duplicate removal, one-candidate
// authorization, semantic duplicate removal, bounds, and exhaustion.
func resolveDatabaseQueryPurposeQueue(
	ctx context.Context,
	authority assemblyline.DatabaseQueryPurposeAuthority,
	maximumAccepted int,
	required bool,
	call objectiveDatabaseRawLeafCall,
	total int,
) ([]string, int, error) {
	if maximumAccepted < 0 {
		return nil, total, fmt.Errorf("database query purpose queue has a negative accepted bound")
	}
	if maximumAccepted == 0 {
		return []string{}, total, nil
	}
	if !required {
		presenceJob, err := assemblyline.NewDatabaseQueryPurposePresenceJob(authority)
		if err != nil {
			return nil, total, err
		}
		presence, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_purpose_presence", presenceJob,
			func(raw string) (assemblyline.DatabaseQueryPurposePresenceResult, error) {
				return assemblyline.DecodeDatabaseQueryPurposePresenceResult(authority, raw)
			},
		)
		total += calls
		if err != nil {
			return nil, total, err
		}
		if !presence.Present {
			return []string{}, total, nil
		}
	}
	inventoryJob, err := assemblyline.NewDatabaseQueryPurposeInventoryJob(authority)
	if err != nil {
		return nil, total, err
	}
	inventory, calls, err := callObjectiveDatabaseRawLeaf(
		ctx, call, "database_query_purpose_inventory", inventoryJob,
		func(raw string) (assemblyline.DatabaseQueryPurposeInventory, error) {
			return assemblyline.DecodeDatabaseQueryPurposeInventory(authority, raw)
		},
	)
	total += calls
	if err != nil {
		return nil, total, err
	}

	accepted := make([]string, 0, len(inventory.Candidates))
	processed := make(map[string]struct{}, len(inventory.Candidates))
	for index, candidate := range inventory.Candidates {
		if _, duplicate := processed[candidate]; duplicate {
			continue
		}
		processed[candidate] = struct{}{}

		necessityInput := assemblyline.DatabaseQueryPurposeNecessityInput{
			Authority: authority, Inventory: inventory, CandidateIndex: index,
		}
		necessityJob, err := assemblyline.NewDatabaseQueryPurposeNecessityJob(necessityInput)
		if err != nil {
			return nil, total, err
		}
		necessity, calls, err := callObjectiveDatabaseRawLeaf(
			ctx, call, "database_query_purpose_necessity", necessityJob,
			func(raw string) (assemblyline.DatabaseQueryPurposeNecessityResult, error) {
				return assemblyline.DecodeDatabaseQueryPurposeNecessityResult(necessityInput, raw)
			},
		)
		total += calls
		if err != nil {
			return nil, total, err
		}
		if necessity.Relation == assemblyline.DatabaseQueryPurposeNotNecessary {
			continue
		}

		duplicate := false
		for _, acceptedPurpose := range accepted {
			relationInput := assemblyline.DatabaseQueryPurposeRelationInput{
				Collection: authority.Collection,
				Candidate:  candidate, AcceptedPurpose: acceptedPurpose,
			}
			relationJob, err := assemblyline.NewDatabaseQueryPurposeRelationJob(relationInput)
			if err != nil {
				return nil, total, err
			}
			relation, relationCalls, err := callObjectiveDatabaseRawLeaf(
				ctx, call, "database_query_purpose_relation", relationJob,
				func(raw string) (assemblyline.DatabaseQueryPurposeRelationResult, error) {
					return assemblyline.DecodeDatabaseQueryPurposeRelationResult(relationInput, raw)
				},
			)
			total += relationCalls
			if err != nil {
				return nil, total, err
			}
			if relation.Relation == assemblyline.DatabaseQueryPurposesSame {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		if len(accepted) == maximumAccepted {
			return nil, total, fmt.Errorf(
				"database query %s purpose queue exceeds its code-owned %d-item bound",
				authority.Collection, maximumAccepted,
			)
		}
		accepted = append(accepted, candidate)
	}
	return accepted, total, nil
}
