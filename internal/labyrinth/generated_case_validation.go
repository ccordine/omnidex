package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func (generated GeneratedCase) validateCoordinates() error {
	descriptor := generated.public.World.Descriptor
	difficulty := descriptor.Difficulty
	corpusCount := 0
	if descriptor.ArtifactCorpus != nil {
		corpusCount = descriptor.ArtifactCorpus.Count
	}
	if len(descriptor.Records)+corpusCount != difficulty.WorldSize ||
		len(generated.oracle.RequiredEvidence) != difficulty.EvidenceArtifacts ||
		len(generated.oracle.Witness) != difficulty.DecisionDepth ||
		len(generated.oracle.CausalDAG) != difficulty.DependencyCount {
		return fmt.Errorf("%w: generated artifacts do not match declared difficulty", ErrGeneration)
	}
	if generated.oracle.GrammarVersion != descriptor.GrammarVersion ||
		generated.oracle.TaskArchetype != archetypeForSuite(descriptor.Suite) {
		return fmt.Errorf("%w: generated suite/version authority is inconsistent", ErrGeneration)
	}
	records := make(map[string]PublicRecord, len(descriptor.Records))
	for _, record := range descriptor.Records {
		records[string(record.ID)] = record
	}
	for _, evidence := range generated.oracle.RequiredEvidence {
		record, exists := records[evidence.ID]
		if !exists || record.ContentSHA256 != evidence.SHA256 {
			return fmt.Errorf("%w: oracle evidence is not bound to a public record", ErrGeneration)
		}
	}
	for index, witness := range generated.oracle.Witness {
		schema, exists := generated.public.World.Catalog.Schema(witness.Request.Kind)
		if !exists || schema.Ref() != witness.Schema {
			return fmt.Errorf("%w: witness action %d is not bound to the public catalog", ErrGeneration, index)
		}
	}
	if !causalEdgesArePublic(generated.oracle.CausalDAG, generated.public.World.InitialFacts) {
		return fmt.Errorf("%w: oracle causal graph is not bound to public topology", ErrGeneration)
	}
	if !generatedTopologyConnected(generated.execution) {
		return fmt.Errorf("%w: generated topology is disconnected", ErrGeneration)
	}
	return verifyGeneratedCausality(generated)
}

func causalEdgesArePublic(edges []CausalEdge, facts []cognition.Predicate) bool {
	public := make(map[string]struct{})
	for _, fact := range facts {
		if fact.Name == "topology.edge" && len(fact.Args) == 2 {
			public[fact.Args[0]+"\x00"+fact.Args[1]] = struct{}{}
		}
	}
	for _, edge := range edges {
		if _, exists := public[string(edge.From)+"\x00"+string(edge.To)]; !exists {
			return false
		}
	}
	return true
}
