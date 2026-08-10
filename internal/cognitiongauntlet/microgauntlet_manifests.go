package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/labyrinth"
)

func (fixture MicrogauntletCase) PublicManifest(surface Surface) (PublicManifest, error) {
	if err := fixture.spec.Validate(); err != nil {
		return PublicManifest{}, err
	}
	if err := fixture.generated.Validate(); err != nil {
		return PublicManifest{}, err
	}
	version, err := surface.Version()
	if err != nil {
		return PublicManifest{}, err
	}
	artifact := fixture.generated.PublicArtifact()
	descriptor := artifact.World.Descriptor
	suite, err := gauntletSuite(descriptor.Suite)
	if err != nil {
		return PublicManifest{}, err
	}
	difficulty := manifestDifficulty(fixture.spec, descriptor.Difficulty, suite)
	manifest := PublicManifest{
		Schema: PublicManifestSchemaV1, Suite: suite, Scenario: artifact.Scenario,
		FormatVersion: descriptor.FormatVersion, SurfaceVersion: version,
		ActionCatalogVersion: artifact.World.Catalog.Version,
		ActionCatalogSHA256:  artifact.World.Catalog.SHA256,
		Goal:                 descriptor.Goal, Difficulty: difficulty,
	}
	return manifest, manifest.Validate()
}

func (fixture MicrogauntletCase) oracleManifest() (OracleManifest, error) {
	oracle := fixture.generated.PrivateOracle()
	quality, err := gauntletOracleQuality(oracle.Quality)
	if err != nil {
		return OracleManifest{}, err
	}
	var optimal *int64
	if oracle.OptimalCost != nil {
		value := int64(*oracle.OptimalCost)
		optimal = &value
	}
	manifest := OracleManifest{
		Schema: OracleManifestSchemaV1, ScenarioID: oracle.ScenarioID,
		PublicSHA256: oracle.PublicSHA256, OracleSHA256: oracle.OracleSHA256,
		GeneratorVersion: oracle.GeneratorVersion, Seed: oracle.Seed, Quality: quality,
		WitnessCost: int64(oracle.WitnessCost), OptimalCost: optimal,
		LowerBound: int64(oracle.LowerBound), TaskArchetype: string(oracle.TaskArchetype),
	}
	return manifest, manifest.Validate()
}

func manifestDifficulty(
	spec MicrogauntletSpec,
	difficulty labyrinth.PublicDifficulty,
	suite Suite,
) Difficulty {
	return Difficulty{
		WorldSize: difficulty.WorldSize, RelevantArtifacts: difficulty.EvidenceArtifacts,
		SolutionDepth: difficulty.DecisionDepth, BranchingFactor: difficulty.BranchingFactor,
		DistractorRatio: float64(difficulty.WorldSize-difficulty.EvidenceArtifacts) / float64(difficulty.WorldSize),
		DependencyCount: difficulty.DependencyCount, DelayedFactCount: delayedFacts(suite),
		SimultaneousGoals: 1, IrreversibleActions: irreversibleActions(suite),
		WorkingSetBudgetBytes: spec.Budget.WorkingSetBytes,
		ContextBudgetBytes:    spec.Budget.ContextBytes, ToolBudget: spec.Budget.ToolOperations,
	}
}

func gauntletSuite(suite labyrinth.Suite) (Suite, error) {
	candidate := Suite(suite)
	switch candidate {
	case SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined:
		return candidate, nil
	default:
		return "", fmt.Errorf("Labyrinth suite %q is not an initial cognition gauntlet", suite)
	}
}

func gauntletOracleQuality(quality labyrinth.OracleQuality) (OracleQuality, error) {
	switch quality {
	case labyrinth.OracleOptimal:
		return OracleOptimal, nil
	case labyrinth.OracleWitnessOnly:
		return OracleWitnessOnly, nil
	default:
		return "", fmt.Errorf("Labyrinth oracle quality %q is not registered", quality)
	}
}

func delayedFacts(suite Suite) int {
	if suite == SuiteRecall || suite == SuiteCombined {
		return 1
	}
	return 0
}

func irreversibleActions(suite Suite) int {
	if suite == SuiteMutate || suite == SuiteCombined {
		return 1
	}
	return 0
}
