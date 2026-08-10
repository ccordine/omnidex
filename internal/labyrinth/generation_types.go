package labyrinth

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

type Suite string

const (
	SuiteRetrieve Suite = "retrieve"
	SuiteRecall   Suite = "recall"
	SuiteUnlock   Suite = "unlock"
	SuiteMutate   Suite = "mutate"
	SuiteCombined Suite = "combined"
	SuiteTraverse Suite = "traverse"
	SuiteBind     Suite = "bind"
	SuiteRevise   Suite = "revise"
	SuiteOrder    Suite = "order"
	SuiteRogue    Suite = "rogue"
)

type Difficulty struct {
	WorldSize         int `json:"world_size"`
	RelevantArtifacts int `json:"relevant_artifacts"`
	SolutionDepth     int `json:"solution_depth"`
	BranchingFactor   int `json:"branching_factor"`
	DependencyCount   int `json:"dependency_count"`
}

type GeneratorConfig struct {
	Suite            Suite      `json:"suite"`
	Seed             uint64     `json:"seed"`
	Difficulty       Difficulty `json:"difficulty"`
	GeneratorVersion string     `json:"generator_version"`
	GrammarVersion   string     `json:"grammar_version"`
	SolverStateLimit int        `json:"solver_state_limit"`
}

// PublicDifficulty preserves declared scaling coordinates without exposing the
// private witness/plan vocabulary used by generation and evaluation.
type PublicDifficulty struct {
	WorldSize         int `json:"world_size"`
	EvidenceArtifacts int `json:"evidence_artifacts"`
	DecisionDepth     int `json:"decision_depth"`
	BranchingFactor   int `json:"branching_factor"`
	DependencyCount   int `json:"dependency_count"`
}

const (
	GeneratorVersionV1          = "generator.v1"
	GrammarVersionV1            = "grammar.v1"
	MinGeneratedWorldSize       = 25
	MaxGeneratedWorldSize       = 250
	MinRelevantArtifacts        = 3
	MaxRelevantArtifacts        = 8
	MinSolutionDepth            = 4
	MaxSolutionDepth            = 10
	MinBranchingFactor          = 1
	MaxBranchingFactor          = 4
	MinDependencyCount          = 2
	MaxDependencyCount          = 5
	MinSolverStateLimit         = 1
	MaxSolverStateLimit         = 100_000
	MaxPublicRecordContentBytes = 512
)

func (suite Suite) Validate() error {
	switch suite {
	case SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined,
		SuiteTraverse, SuiteBind, SuiteRevise, SuiteOrder, SuiteRogue:
		return nil
	default:
		return fmt.Errorf("%w: suite %q is not registered", ErrInvalidGeneratorConfig, suite)
	}
}

func (difficulty Difficulty) Validate() error {
	if difficulty.WorldSize < MinGeneratedWorldSize || difficulty.WorldSize > MaxGeneratedWorldSize {
		return fmt.Errorf("%w: world size is outside [%d,%d]", ErrInvalidGeneratorConfig, MinGeneratedWorldSize, MaxGeneratedWorldSize)
	}
	if difficulty.RelevantArtifacts < 1 || difficulty.RelevantArtifacts > MaxRelevantArtifacts ||
		difficulty.RelevantArtifacts > difficulty.WorldSize {
		return fmt.Errorf("%w: relevant artifact count is invalid", ErrInvalidGeneratorConfig)
	}
	if difficulty.SolutionDepth < MinSolutionDepth || difficulty.SolutionDepth > MaxSolutionDepth {
		return fmt.Errorf("%w: solution depth is outside [%d,%d]", ErrInvalidGeneratorConfig, MinSolutionDepth, MaxSolutionDepth)
	}
	if difficulty.BranchingFactor < MinBranchingFactor || difficulty.BranchingFactor > MaxBranchingFactor {
		return fmt.Errorf("%w: branching factor is outside [%d,%d]", ErrInvalidGeneratorConfig, MinBranchingFactor, MaxBranchingFactor)
	}
	if difficulty.DependencyCount < MinDependencyCount || difficulty.DependencyCount > MaxDependencyCount ||
		difficulty.DependencyCount > difficulty.SolutionDepth {
		return fmt.Errorf("%w: dependency count is invalid", ErrInvalidGeneratorConfig)
	}
	return nil
}

func (difficulty Difficulty) public() PublicDifficulty {
	return PublicDifficulty{
		WorldSize: difficulty.WorldSize, EvidenceArtifacts: difficulty.RelevantArtifacts,
		DecisionDepth: difficulty.SolutionDepth, BranchingFactor: difficulty.BranchingFactor,
		DependencyCount: difficulty.DependencyCount,
	}
}

func (difficulty PublicDifficulty) Validate() error {
	if difficulty.WorldSize < MinGeneratedWorldSize || difficulty.WorldSize > MaxScaleWorldSize {
		return fmt.Errorf("%w: public world size is outside [%d,%d]", ErrInvalidGeneratorConfig, MinGeneratedWorldSize, MaxScaleWorldSize)
	}
	return (Difficulty{
		WorldSize: MinGeneratedWorldSize, RelevantArtifacts: difficulty.EvidenceArtifacts,
		SolutionDepth: difficulty.DecisionDepth, BranchingFactor: difficulty.BranchingFactor,
		DependencyCount: difficulty.DependencyCount,
	}).Validate()
}

func (config GeneratorConfig) Validate() error {
	if err := config.Suite.Validate(); err != nil {
		return err
	}
	if err := config.Difficulty.Validate(); err != nil {
		return err
	}
	if config.Suite != SuiteRecall && config.Difficulty.RelevantArtifacts < MinRelevantArtifacts {
		return fmt.Errorf("%w: suite requires at least %d relevant artifacts", ErrInvalidGeneratorConfig, MinRelevantArtifacts)
	}
	if config.GeneratorVersion != GeneratorVersionV1 || config.GrammarVersion != GrammarVersionV1 {
		return fmt.Errorf("%w: generator or grammar version is not registered", ErrInvalidGeneratorConfig)
	}
	if config.SolverStateLimit < MinSolverStateLimit || config.SolverStateLimit > MaxSolverStateLimit {
		return fmt.Errorf("%w: solver state limit is outside [%d,%d]", ErrInvalidGeneratorConfig, MinSolverStateLimit, MaxSolverStateLimit)
	}
	return nil
}

type PublicRecord struct {
	ID            EntityID `json:"id"`
	Location      EntityID `json:"location"`
	Content       string   `json:"content"`
	ContentSHA256 string   `json:"content_sha256"`
}

type PublicDescriptor struct {
	Suite          Suite              `json:"suite"`
	FormatVersion  string             `json:"format_version"`
	SurfaceVersion string             `json:"surface_version"`
	GrammarVersion string             `json:"grammar_version"`
	Goal           string             `json:"goal"`
	Difficulty     PublicDifficulty   `json:"difficulty"`
	Records        []PublicRecord     `json:"records"`
	ArtifactCorpus *ArtifactCorpusRef `json:"artifact_corpus,omitempty"`
}

type PublicWorld struct {
	Schema           string                  `json:"schema"`
	ScenarioID       cognition.ScenarioID    `json:"scenario_id"`
	Descriptor       PublicDescriptor        `json:"descriptor"`
	Catalog          cognition.ActionCatalog `json:"action_catalog"`
	Entities         []Entity                `json:"entities"`
	PredicateSchemas []PredicateSchema       `json:"predicate_schemas"`
	InitialFacts     []cognition.Predicate   `json:"initial_facts"`
}
