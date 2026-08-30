package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialists"
)

func TestLearnedProcedureAppearsOnlyInImplementationContracts(t *testing.T) {
	version := validDirectCodingSkillVersion(t, specialists.SkillStatusActive)
	procedure := version.Spec.Instructions
	bindings := func(requirementID string) map[string]directCodingSkillBinding {
		return map[string]directCodingSkillBinding{
			requirementID: {
				RequirementID: requirementID,
				Procedure:     procedure,
				Version:       version,
			},
		}
	}

	fixtures := []struct {
		name     string
		language string
		compile  func(*testing.T) (assemblyline.SourceBlueprint, error)
	}{
		{
			name: "go", language: "go",
			compile: func(t *testing.T) (assemblyline.SourceBlueprint, error) {
				specification, workload := goCommandLineStackFixture(t)
				target, coverage := learnedProcedureTestTargetAndCoverage(
					t, workload, genericGoCommandLineAdapter, goCommandLineVersionProfileV1,
					"feature.go", "feature_test.go",
				)
				blueprint, _, err := compileGenericGoCommandLineBlueprint(
					"skill-projection", specification, bindings("requirement_001"), workload,
					directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
				)
				return blueprint, err
			},
		},
		{
			name: "javascript", language: "javascript",
			compile: func(t *testing.T) (assemblyline.SourceBlueprint, error) {
				specification, workload := javaScriptCommandLineStackFixture(t)
				target, coverage := learnedProcedureTestTargetAndCoverage(
					t, workload, genericJavaScriptCommandLineAdapter,
					javaScriptCommandLineVersionProfileV1, "feature.mjs", "feature.test.mjs",
				)
				blueprint, _, err := compileGenericJavaScriptCommandLineBlueprint(
					"skill-projection", specification, bindings("requirement_001"), workload,
					directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
				)
				return blueprint, err
			},
		},
		{
			name: "java", language: "java",
			compile: func(t *testing.T) (assemblyline.SourceBlueprint, error) {
				specification, workload := javaCommandLineStackFixture(t)
				target, coverage := learnedProcedureTestTargetAndCoverage(
					t, workload, genericJavaCommandLineAdapter, javaCommandLineVersionProfileV1,
					"Feature001.java", "FeatureTest001.java",
				)
				blueprint, _, err := compileGenericJavaCommandLineBlueprint(
					"skill-projection", specification, bindings("requirement_001"), workload,
					directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
				)
				return blueprint, err
			},
		},
		{
			name: "rust", language: "rust",
			compile: func(t *testing.T) (assemblyline.SourceBlueprint, error) {
				specification, workload := rustCommandLineStackFixture(t)
				target, coverage := learnedProcedureTestTargetAndCoverage(
					t, workload, genericRustCommandLineAdapter, rustCommandLineVersionProfileV1,
					"src/feature.rs", "tests/feature_test.rs",
				)
				blueprint, _, err := compileGenericRustCommandLineBlueprint(
					"skill-projection", specification, bindings("requirement_001"), workload,
					directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
				)
				return blueprint, err
			},
		},
		{
			name: "php", language: "php",
			compile: func(t *testing.T) (assemblyline.SourceBlueprint, error) {
				specification, workload, target, coverage, endpoints := phpServiceStackFixture(t)
				blueprint, _, err := compileGenericPHPServiceBlueprint(
					"skill-projection", specification, bindings("requirement_001"), workload,
					directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
					testRequestLocalServiceStatePlan(workload), endpoints,
				)
				return blueprint, err
			},
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			blueprint, err := fixture.compile(t)
			if err != nil {
				t.Fatal(err)
			}
			implementationMatches := 0
			for _, document := range blueprint.Documents {
				for _, block := range document.Blocks {
					if block.Contract == "" {
						continue
					}
					job, err := assemblyline.NewFragmentGenerationJob(
						assemblyline.FragmentGenerationInput{
							Language: fixture.language, Dialect: fixture.language + " bounded source",
							Signature: block.Signature, Behavior: block.Contract,
							PermittedSymbols: append([]string(nil), block.Globals...),
						},
					)
					if err != nil {
						t.Fatalf("build %s model envelope: %v", block.ID, err)
					}
					prompt, err := assemblyline.RenderPortableJob(job)
					if err != nil {
						t.Fatalf("render %s model envelope: %v", block.ID, err)
					}
					contractHasProcedure := strings.Contains(block.Contract, procedure)
					promptHasProcedure := strings.Contains(prompt, procedure)
					if contractHasProcedure != promptHasProcedure {
						t.Fatalf("rendered %s prompt changed learned-procedure projection", block.ID)
					}
					if !contractHasProcedure {
						continue
					}
					if block.Role != assemblyline.SourceBlockTaskImplementation {
						t.Fatalf(
							"learned procedure reached %s block %s:\n%s",
							block.Role, block.ID, block.Contract,
						)
					}
					if strings.Count(block.Contract, procedure) != 1 ||
						strings.Count(prompt, procedure) != 1 {
						t.Fatalf("implementation block %s or its prompt repeats learned procedure", block.ID)
					}
					implementationMatches++
				}
			}
			if implementationMatches != 1 {
				t.Fatalf("implementation contracts containing learned procedure=%d want 1", implementationMatches)
			}
		})
	}
}

func learnedProcedureTestTargetAndCoverage(
	t *testing.T,
	workload assemblyline.FrozenApplicationWorkload,
	stackID string,
	versionProfileID string,
	implementationPath string,
	verificationPath string,
) (assemblyline.TargetTree, assemblyline.ApplicationFileCoveragePlan) {
	t.Helper()
	target := assemblyline.TargetTree{
		StackID: stackID, VersionProfileID: versionProfileID,
		Paths: []string{implementationPath, verificationPath},
	}
	taskID := workload.Tasks[0].ID
	coverage, err := assemblyline.NewApplicationFileCoveragePlan(
		workload,
		target,
		map[string][]string{
			implementationPath: {taskID},
			verificationPath:   {taskID},
		},
		map[string]assemblyline.TargetArtifactKind{
			implementationPath: assemblyline.TargetArtifactImplementation,
			verificationPath:   assemblyline.TargetArtifactVerification,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return target, coverage
}
