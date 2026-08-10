package cognitiongauntlet

import "github.com/gryph/omnidex/internal/labyrinth"

func InitialMicrogauntletsV1() []MicrogauntletSpec {
	return []MicrogauntletSpec{
		microSpec("retrieve-v1", labyrinth.SuiteRetrieve, 11_001, labyrinth.Difficulty{
			WorldSize: 40, RelevantArtifacts: 3, SolutionDepth: 4,
			BranchingFactor: 2, DependencyCount: 2,
		}),
		microSpec("recall-v1", labyrinth.SuiteRecall, 12_001, labyrinth.Difficulty{
			WorldSize: 48, RelevantArtifacts: 4, SolutionDepth: 6,
			BranchingFactor: 2, DependencyCount: 2,
		}),
		microSpec("unlock-v1", labyrinth.SuiteUnlock, 13_001, labyrinth.Difficulty{
			WorldSize: 56, RelevantArtifacts: 4, SolutionDepth: 6,
			BranchingFactor: 3, DependencyCount: 4,
		}),
		microSpec("mutate-v1", labyrinth.SuiteMutate, 14_001, labyrinth.Difficulty{
			WorldSize: 48, RelevantArtifacts: 3, SolutionDepth: 5,
			BranchingFactor: 2, DependencyCount: 3,
		}),
		microSpec("combined-v1", labyrinth.SuiteCombined, 15_001, labyrinth.Difficulty{
			WorldSize: 64, RelevantArtifacts: 5, SolutionDepth: 7,
			BranchingFactor: 3, DependencyCount: 5,
		}),
	}
}

func microSpec(
	id string,
	suite labyrinth.Suite,
	seed uint64,
	difficulty labyrinth.Difficulty,
) MicrogauntletSpec {
	return MicrogauntletSpec{
		CaseID: "micro-" + id, FixtureVersion: InitialMicrogauntletFixtureVersionV1,
		Generator: labyrinth.GeneratorConfig{
			Suite: suite, Seed: seed, Difficulty: difficulty,
			GeneratorVersion: labyrinth.GeneratorVersionV1,
			GrammarVersion:   labyrinth.GrammarVersionV1, SolverStateLimit: 100_000,
		},
		Budget: RunBudget{
			ContextBytes: 24_576, WorkingSetBytes: 8_192, RuntimeCycles: 96, ModelCalls: 32,
			EnvironmentActions: 64, ToolOperations: 64,
			Station: StationBudget{
				MaxInputBytes: 24_576, MaxInputTokens: 6_144,
				MaxOutputBytes: 4_096, MaxOutputTokens: 1_024,
			},
			Decision: DecisionBudget{
				MaxEvidenceRefs: 16, MaxActionArguments: 8,
				MaxLedgerProposals: 8, MaxAttentionRequests: 8,
				MaxExpectedEffectBytes: 1024,
			},
		},
	}
}
