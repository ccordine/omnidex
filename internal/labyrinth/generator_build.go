package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

type generatedWorldParts struct {
	entities []Entity
	facts    []cognition.Predicate
	records  []PublicRecord
	evidence []EvidenceIdentity
	contract causalContract
}

func buildGeneratedWorld(
	config GeneratorConfig,
	plan causalPlan,
	random *deterministicRandom,
) (generatedWorldParts, error) {
	entities, stages, edges := buildStageTopology(config, plan)
	facts := make([]cognition.Predicate, 0, len(edges)+config.Difficulty.WorldSize+32)
	for _, edge := range edges {
		predicate, err := generatedPredicate("topology.edge", edge.From, edge.To)
		if err != nil {
			return generatedWorldParts{}, err
		}
		facts = append(facts, predicate)
	}
	initial, err := generatedPredicate("state.current", plan.mainStages[0])
	if err != nil {
		return generatedWorldParts{}, err
	}
	marker, err := generatedPredicate("surface.marker", plan.mainStages[0])
	if err != nil {
		return generatedWorldParts{}, err
	}
	facts = append(facts, initial, marker)
	recordEntities, records, evidence, contract, err := buildRecords(config, plan, stages, random)
	if err != nil {
		return generatedWorldParts{}, err
	}
	entities = append(entities, recordEntities...)
	for _, record := range records {
		located, locateErr := generatedPredicate("record.at", record.ID, record.Location)
		if locateErr != nil {
			return generatedWorldParts{}, locateErr
		}
		facts = append(facts, located)
	}
	contractEntities, contractFacts, err := buildContractState(contract, plan)
	if err != nil {
		return generatedWorldParts{}, err
	}
	entities = append(entities, contractEntities...)
	facts = append(facts, contractFacts...)
	return generatedWorldParts{entities, facts, records, evidence, contract}, nil
}

func buildStageTopology(config GeneratorConfig, plan causalPlan) ([]Entity, []EntityID, []CausalEdge) {
	stages := append([]EntityID(nil), plan.mainStages...)
	edges := make([]CausalEdge, 0, config.Difficulty.SolutionDepth*(1+2*config.Difficulty.BranchingFactor))
	for index := 0; index < config.Difficulty.SolutionDepth; index++ {
		edges = append(edges, CausalEdge{From: plan.mainStages[index], To: plan.mainStages[index+1]})
		for branch := 0; branch < config.Difficulty.BranchingFactor; branch++ {
			side := EntityID(fmt.Sprintf("branch-%03d-%02d", index, branch))
			stages = append(stages, side)
			edges = append(edges,
				CausalEdge{From: plan.mainStages[index], To: side},
				CausalEdge{From: side, To: plan.mainStages[index]},
			)
		}
	}
	entities := make([]Entity, len(stages))
	for index, stage := range stages {
		entities[index] = Entity{ID: stage, Kind: stageKind, Public: true}
	}
	return entities, stages, edges
}

func buildRecords(
	config GeneratorConfig,
	plan causalPlan,
	stages []EntityID,
	random *deterministicRandom,
) ([]Entity, []PublicRecord, []EvidenceIdentity, causalContract, error) {
	count := config.Difficulty.WorldSize
	permutation := random.permutation(count)
	targetIndex := config.Difficulty.SolutionDepth - 1
	relevantOrder := make([]int, 0, config.Difficulty.RelevantArtifacts)
	for _, candidate := range permutation {
		if candidate == targetIndex || candidate > 0 && candidate < config.Difficulty.SolutionDepth {
			continue
		}
		relevantOrder = append(relevantOrder, candidate)
		if len(relevantOrder) == config.Difficulty.RelevantArtifacts {
			break
		}
	}
	relevant := make(map[int]int, len(relevantOrder))
	requiredRecords := make([]EntityID, len(relevantOrder))
	for rank, index := range relevantOrder {
		relevant[index] = rank
		requiredRecords[rank] = EntityID(fmt.Sprintf("record-%03d", index))
	}
	contractRecords := requiredRecords
	if config.Suite == SuiteRecall {
		contractRecords = requiredRecords[:1]
	}
	contract, err := buildCausalContract(config, plan, contractRecords)
	if err != nil {
		return nil, nil, nil, causalContract{}, err
	}
	entities := make([]Entity, count)
	records := make([]PublicRecord, count)
	evidence := make([]EvidenceIdentity, 0, len(relevant))
	for index := 0; index < count; index++ {
		id := EntityID(fmt.Sprintf("record-%03d", index))
		entities[index] = Entity{ID: id, Kind: recordKind, Public: true}
		location := stages[random.index(len(stages))]
		if index < len(plan.mainStages) {
			location = plan.mainStages[index]
		}
		rank, required := relevant[index]
		if id == contract.object {
			location = plan.locationForKind("take")
		}
		if id == contract.mutationTarget {
			location = plan.locationForKind("write")
		}
		content := generatedRecordContent(index, rank, required, plan, contract, random)
		record, err := NewPublicRecord(id, location, content)
		if err != nil {
			return nil, nil, nil, causalContract{}, err
		}
		records[index] = record
		if containsEntityID(contract.requiredRecords, id) {
			evidence = append(evidence, EvidenceIdentity{ID: string(id), SHA256: record.ContentSHA256})
		}
	}
	return entities, records, evidence, contract, nil
}

func generatedRecordContent(
	index, rank int,
	required bool,
	plan causalPlan,
	contract causalContract,
	random *deterministicRandom,
) string {
	if EntityID(fmt.Sprintf("record-%03d", index)) == contract.mutationTarget {
		return generatedMutationTargetContent(index)
	}
	if required {
		step := rank % len(plan.macroKinds)
		return fmt.Sprintf(
			"Entry %03d matches query %s; operation %s uses evidence set %s. Object %s, target %s, expected hash %s, and exact value %s are registered with token %016x.",
			index, contract.query, plan.macroKinds[step], contract.evidenceSet,
			contract.object, contract.mutationTarget, contract.mutationExpected,
			contract.mutationValue, random.next(),
		)
	}
	kind := v1MacroKinds[random.index(len(v1MacroKinds))]
	return fmt.Sprintf(
		"Entry %03d archives checkpoint %03d with operation %s and token %016x.",
		index, random.index(len(plan.macroKinds)), kind, random.next(),
	)
}

func generatedMutationTargetContent(index int) string {
	return fmt.Sprintf("Mutable entry %03d is awaiting one exact hash-bound value.", index)
}

func generatedPredicate(name cognition.PredicateName, entities ...EntityID) (cognition.Predicate, error) {
	arguments := make([]string, len(entities))
	for index, entity := range entities {
		arguments[index] = string(entity)
	}
	predicate, err := cognition.NewPredicate(name, arguments)
	if err != nil {
		return cognition.Predicate{}, fmt.Errorf("%w: construct predicate %s: %v", ErrGeneration, name, err)
	}
	return predicate, nil
}
