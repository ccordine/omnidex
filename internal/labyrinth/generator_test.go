package labyrinth

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestGenerateBuildsEveryV1SuiteFromAVerifiedWitness(t *testing.T) {
	t.Parallel()
	for _, suite := range []Suite{SuiteRetrieve, SuiteRecall, SuiteUnlock, SuiteMutate, SuiteCombined} {
		suite := suite
		t.Run(string(suite), func(t *testing.T) {
			t.Parallel()
			generated, err := Generate(testGeneratorConfig(suite, 41))
			if err != nil {
				t.Fatalf("generate: %v", err)
			}
			if err := generated.Validate(); err != nil {
				t.Fatalf("validate generated case: %v", err)
			}
			transition, cost, err := VerifyWitness(generated)
			if err != nil {
				t.Fatalf("verify witness: %v", err)
			}
			oracle := generated.PrivateOracle()
			if !transition.Terminal || cost != oracle.WitnessCost {
				t.Fatalf("witness terminal=%t cost=%d, oracle cost=%d", transition.Terminal, cost, oracle.WitnessCost)
			}
			if oracle.TaskArchetype != archetypeForSuite(suite) {
				t.Fatalf("archetype = %q", oracle.TaskArchetype)
			}
		})
	}
}

func TestGenerateIsDeterministicAndSeparatesArtifacts(t *testing.T) {
	t.Parallel()
	config := testGeneratorConfig(SuiteCombined, 918273)
	first, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Generate(config)
	if err != nil {
		t.Fatal(err)
	}
	firstPublic, _ := first.MarshalPublicJSON()
	secondPublic, _ := second.MarshalPublicJSON()
	firstOracle, _ := first.MarshalOracleJSON()
	secondOracle, _ := second.MarshalOracleJSON()
	if !bytes.Equal(firstPublic, secondPublic) || !bytes.Equal(firstOracle, secondOracle) {
		t.Fatal("identical configuration produced different artifacts")
	}
	if _, err := json.Marshal(first); !errors.Is(err, ErrArtifactSeparation) {
		t.Fatalf("aggregate marshal error = %v, want ErrArtifactSeparation", err)
	}
	publicText := strings.ToLower(string(firstPublic))
	for _, forbidden := range []string{"\"seed\"", "oracle", "solution", "relevance", "latent.", "private."} {
		if strings.Contains(publicText, forbidden) {
			t.Fatalf("public artifact contains forbidden private term %q", forbidden)
		}
	}
}

func TestGeneratorRejectsInvalidLimitsWithoutPartialArtifacts(t *testing.T) {
	t.Parallel()
	config := testGeneratorConfig(SuiteRetrieve, 1)
	config.Difficulty.SolutionDepth = MaxSolutionDepth + 1
	generated, err := Generate(config)
	if !errors.Is(err, ErrInvalidGeneratorConfig) {
		t.Fatalf("error = %v, want ErrInvalidGeneratorConfig", err)
	}
	if generated.execution.ref.ID != "" || generated.public.Schema != "" || generated.oracle.Schema != "" {
		t.Fatal("invalid generation returned a partial case")
	}
}

func TestGeneratorRejectsEveryInvalidAuthorityCoordinate(t *testing.T) {
	t.Parallel()
	tests := map[string]func(*GeneratorConfig){
		"suite":             func(config *GeneratorConfig) { config.Suite = "unknown" },
		"world":             func(config *GeneratorConfig) { config.Difficulty.WorldSize = MinGeneratedWorldSize - 1 },
		"evidence count":    func(config *GeneratorConfig) { config.Difficulty.RelevantArtifacts = 0 },
		"decision depth":    func(config *GeneratorConfig) { config.Difficulty.SolutionDepth = MaxSolutionDepth + 1 },
		"branching":         func(config *GeneratorConfig) { config.Difficulty.BranchingFactor = 0 },
		"dependencies":      func(config *GeneratorConfig) { config.Difficulty.DependencyCount = 0 },
		"generator version": func(config *GeneratorConfig) { config.GeneratorVersion = "" },
		"grammar version":   func(config *GeneratorConfig) { config.GrammarVersion = "invalid version" },
		"unknown version":   func(config *GeneratorConfig) { config.GeneratorVersion = "generator.v2" },
		"solver bound":      func(config *GeneratorConfig) { config.SolverStateLimit = 0 },
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			config := testGeneratorConfig(SuiteRetrieve, 1)
			mutate(&config)
			if _, err := Generate(config); !errors.Is(err, ErrInvalidGeneratorConfig) {
				t.Fatalf("error = %v, want ErrInvalidGeneratorConfig", err)
			}
		})
	}
}

func TestGeneratorAcceptsItsDeclaredMaximumBoundary(t *testing.T) {
	t.Parallel()
	config := testGeneratorConfig(SuiteCombined, ^uint64(0))
	config.Difficulty = Difficulty{
		WorldSize: MaxGeneratedWorldSize, RelevantArtifacts: MaxRelevantArtifacts,
		SolutionDepth: MaxSolutionDepth, BranchingFactor: MaxBranchingFactor,
		DependencyCount: MaxDependencyCount,
	}
	config.SolverStateLimit = MaxSolverStateLimit
	generated, err := Generate(config)
	if err != nil {
		t.Fatalf("generate declared maximum: %v", err)
	}
	if _, err := generated.MarshalPublicJSON(); err != nil {
		t.Fatalf("marshal declared maximum: %v", err)
	}
}

func TestGeneratedCaseRejectsOracleOutsideDeclaredPublicCoordinates(t *testing.T) {
	t.Parallel()
	generated, err := Generate(testGeneratorConfig(SuiteUnlock, 65))
	if err != nil {
		t.Fatal(err)
	}
	tampered := generated
	tampered.oracle = generated.oracle.clone()
	additional := generated.public.World.Descriptor.Records[len(generated.oracle.RequiredEvidence)]
	additionalEvidence := EvidenceIdentity{
		ID: string(additional.ID), SHA256: additional.ContentSHA256,
	}
	tampered.oracle.RequiredEvidence = append(tampered.oracle.RequiredEvidence, additionalEvidence)
	tampered.oracle.EvidenceUses = append(tampered.oracle.EvidenceUses, EvidenceUse{
		Evidence:            additionalEvidence,
		AcquisitionActionID: tampered.oracle.EvidenceUses[0].AcquisitionActionID,
		RequiredByActionID:  tampered.oracle.EvidenceUses[0].RequiredByActionID,
	})
	tampered.oracle.OracleSHA256 = ""
	if err := tampered.oracle.seal(); err != nil {
		t.Fatal(err)
	}
	if err := tampered.Validate(); !errors.Is(err, ErrGeneration) {
		t.Fatalf("error = %v, want generated-coordinate rejection", err)
	}
}

func testGeneratorConfig(suite Suite, seed uint64) GeneratorConfig {
	return GeneratorConfig{
		Suite: suite,
		Seed:  seed,
		Difficulty: Difficulty{
			WorldSize: 25, RelevantArtifacts: 3, SolutionDepth: 4,
			BranchingFactor: 2, DependencyCount: 2,
		},
		GeneratorVersion: GeneratorVersionV1,
		GrammarVersion:   GrammarVersionV1,
		SolverStateLimit: 5000,
	}
}
