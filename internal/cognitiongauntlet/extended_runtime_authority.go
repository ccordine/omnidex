package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

func extendedRuntimeAuthority(
	generated labyrinth.ExtendedCase,
	request ExtendedRuntimeRunRequest,
	allowRogue bool,
) (PairedRunAuthority, error) {
	if err := generated.Validate(); err != nil {
		return PairedRunAuthority{}, err
	}
	if err := request.Validate(); err != nil {
		return PairedRunAuthority{}, err
	}
	public := generated.PublicArtifact()
	oracle := generated.PrivateOracle()
	suite := Suite(public.World.Descriptor.Suite)
	ordinary := containsSuite([]Suite{SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder}, suite)
	if !ordinary && !(allowRogue && suite == SuiteRogue) {
		return PairedRunAuthority{}, fmt.Errorf("extended runtime suite %q is not admitted", suite)
	}
	surfaceVersion, err := request.Surface.Version()
	if err != nil {
		return PairedRunAuthority{}, err
	}
	budget := extendedRuntimeBudget()
	authority := PairedRunAuthority{
		Schema: PairedRunAuthoritySchemaV1,
		CaseID: "extended-" + string(suite) + "-v1", Suite: suite,
		FixtureVersion:   ExtendedSuiteFixtureVersionV1,
		GeneratorVersion: oracle.GeneratorVersion, Seed: oracle.Seed,
		Scenario: public.Scenario, OracleSHA256: oracle.OracleSHA256,
		SurfaceVersion: surfaceVersion, ActionCatalogVersion: public.World.Catalog.Version,
		ActionCatalogSHA256: public.World.Catalog.SHA256,
		RatGeneration:       request.RatGeneration, Budget: budget,
		Runtime: request.RuntimeFingerprint, Repetition: request.Repetition,
	}
	if len(oracle.Witness) > budget.EnvironmentActions || len(oracle.Witness) > budget.ModelCalls {
		return PairedRunAuthority{}, fmt.Errorf("extended witness exceeds its frozen runtime budget")
	}
	return authority, authority.Validate()
}

func extendedRuntimeBudget() RunBudget {
	return RunBudget{
		ContextBytes: 24_576, WorkingSetBytes: 8_192,
		RuntimeCycles: 96, ModelCalls: 32, EnvironmentActions: 64, ToolOperations: 64,
		Station: StationBudget{
			MaxInputBytes: 24_576, MaxInputTokens: 6_144,
			MaxOutputBytes: 4_096, MaxOutputTokens: 1_024,
		},
		Decision: DecisionBudget{
			MaxEvidenceRefs: 16, MaxActionArguments: 8,
			MaxLedgerProposals: 8, MaxAttentionRequests: 8,
			MaxExpectedEffectBytes: 1_024,
		},
	}
}
