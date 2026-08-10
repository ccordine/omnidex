package labyrinth

import "github.com/gryph/omnidex/internal/cognition"

func buildContractState(
	contract causalContract,
	plan causalPlan,
) ([]Entity, []cognition.Predicate, error) {
	entities := []Entity{
		{ID: contract.evidenceSet, Kind: evidenceSetKind},
		{ID: contract.query, Kind: queryKind, Public: true},
		{ID: contract.queryDecoy, Kind: queryKind},
		{ID: contract.mutationValue, Kind: mutationValueKind},
		{ID: contract.mutationDecoy, Kind: mutationValueKind},
		{ID: contract.mutationExpected, Kind: contentHashKind},
		{ID: contract.mutationCurrent, Kind: contentHashKind},
	}
	facts := make([]cognition.Predicate, 0, len(contract.requiredRecords)+10)
	for _, record := range contract.requiredRecords {
		member, err := generatedPredicate("evidence.member", contract.evidenceSet, record)
		if err != nil {
			return nil, nil, err
		}
		facts = append(facts, member)
	}
	coordinates := []struct {
		name     cognition.PredicateName
		entities []EntityID
	}{
		{"surface.query", []EntityID{contract.query, plan.locationForKind("search")}},
		{"surface.focus", []EntityID{contract.readArtifact, plan.locationForKind("read")}},
		{"objective.read", []EntityID{contract.readArtifact}},
		{"objective.object", []EntityID{contract.object}},
		{"objective.use", []EntityID{contract.object, contract.useTarget}},
		{"write.allowed", []EntityID{contract.mutationTarget, contract.mutationExpected, contract.mutationValue}},
		{"record.content_hash", []EntityID{contract.mutationTarget, contract.mutationExpected}},
	}
	for _, coordinate := range coordinates {
		fact, err := generatedPredicate(coordinate.name, coordinate.entities...)
		if err != nil {
			return nil, nil, err
		}
		facts = append(facts, fact)
	}
	return entities, facts, nil
}
