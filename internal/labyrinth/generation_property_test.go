package labyrinth

import (
	"bytes"
	"strings"
	"testing"
)

func TestGeneratedV1WorldsRemainSolvableAcrossThousandsOfSeeds(t *testing.T) {
	if testing.Short() {
		t.Skip("2,000-case deterministic generation property rail")
	}
	suites := []Suite{SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined}
	for suiteIndex, suite := range suites {
		for seed := uint64(1); seed <= 400; seed++ {
			config := testGeneratorConfig(suite, seed+uint64(suiteIndex)*10_000)
			generated, err := Generate(config)
			if err != nil {
				t.Fatalf("suite=%s seed=%d: %v", suite, seed, err)
			}
			if _, _, err := VerifyWitness(generated); err != nil {
				t.Fatalf("suite=%s seed=%d witness: %v", suite, seed, err)
			}
			oracle := generated.PrivateOracle()
			if oracle.Quality != OracleOptimal || oracle.OptimalCost == nil ||
				*oracle.OptimalCost > oracle.WitnessCost {
				t.Fatalf("suite=%s seed=%d lacks an exact exhaustive cost proof", suite, seed)
			}
			if !generatedTopologyConnected(generated.ExecutionScenario()) {
				t.Fatalf("suite=%s seed=%d topology is disconnected", suite, seed)
			}
			public, marshalErr := generated.MarshalPublicJSON()
			if marshalErr != nil {
				t.Fatalf("suite=%s seed=%d public artifact: %v", suite, seed, marshalErr)
			}
			for _, forbidden := range []string{
				"\"seed\"", "oracle", "solution", "relevance", "latent.", "private.",
				"evidence.required", "evidence.distractor", "state.current", "state.completed", "permit.",
			} {
				if strings.Contains(strings.ToLower(string(public)), forbidden) {
					t.Fatalf("suite=%s seed=%d public artifact contains %q", suite, seed, forbidden)
				}
			}
			repeated, repeatErr := generateWithoutSolve(config)
			if repeatErr != nil {
				t.Fatal(repeatErr)
			}
			repeatedPublic, _ := repeated.MarshalPublicJSON()
			if !bytes.Equal(public, repeatedPublic) ||
				canonicalJSON(staticOracleIdentity(generated.oracle)) != canonicalJSON(staticOracleIdentity(repeated.oracle)) ||
				generated.public.World.Descriptor.Goal != repeated.public.World.Descriptor.Goal {
				t.Fatalf("suite=%s seed=%d is nondeterministic", suite, seed)
			}
		}
	}
}

func staticOracleIdentity(oracle Oracle) any {
	return struct {
		ScenarioID       string
		PublicSHA256     string
		GeneratorVersion string
		GrammarVersion   string
		Seed             uint64
		DefinitionSHA256 string
		Witness          []WitnessAction
		WitnessCost      int
		RequiredEvidence []EvidenceIdentity
		EvidenceUses     []EvidenceUse
		CausalDAG        []CausalEdge
		TaskArchetype    TaskArchetype
	}{
		string(oracle.ScenarioID), oracle.PublicSHA256, oracle.GeneratorVersion,
		oracle.GrammarVersion, oracle.Seed, oracle.DefinitionSHA256, cloneWitness(oracle.Witness),
		oracle.WitnessCost, append([]EvidenceIdentity(nil), oracle.RequiredEvidence...),
		append([]EvidenceUse(nil), oracle.EvidenceUses...),
		append([]CausalEdge(nil), oracle.CausalDAG...), oracle.TaskArchetype,
	}
}
