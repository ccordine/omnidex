package worker

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGoSelectedRuntimeCapabilitiesCompileRunAndRemainTaskScoped(t *testing.T) {
	fixtures := []struct {
		name           string
		selectedID     string
		selectedAPI    string
		unselectedAPI  string
		implementation string
		acceptance     string
		forbidden      string
		forbiddenName  string
		prepare        func(*testing.T, string)
	}{
		{
			name:          "local poem source",
			selectedID:    "runtime.stdlib.read_file",
			selectedAPI:   "RuntimeReadFile",
			unselectedAPI: "RuntimeEnvironmentValue",
			implementation: `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	if len(input.Arguments) != 1 {
		return TaskResult{Error: "one poem name is required", ExitCode: 2}
	}
	contents, failure := RuntimeReadFile(input.Arguments[0])
	if failure != nil {
		return TaskResult{Error: "poem could not be read", ExitCode: 1}
	}
	return TaskResult{Output: string(contents)}
}`,
			acceptance: `func TestFeature001(t *testing.T) {
	result := Feature001(TaskInput{Arguments: []string{"poem.fixture"}}, CapabilityResults{})
	if result.Output != "quiet river" {
		t.Fatalf("poem = %q error = %q", result.Output, result.Error)
	}
}`,
			forbidden: `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	contents, _ := os.ReadFile("poem.fixture")
	return TaskResult{Output: string(contents)}
}`,
			forbiddenName: "ReadFile",
			prepare: func(t *testing.T, root string) {
				t.Helper()
				if err := os.WriteFile(
					filepath.Join(root, "poem.fixture"), []byte("quiet river"), 0o600,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:          "isolated home discovery",
			selectedID:    "runtime.stdlib.environment_value",
			selectedAPI:   "RuntimeEnvironmentValue",
			unselectedAPI: "RuntimeReadFile",
			implementation: `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	if len(input.Arguments) != 1 {
		return TaskResult{Error: "one region key is required", ExitCode: 2}
	}
	region, exists := RuntimeEnvironmentValue(input.Arguments[0])
	if !exists {
		return TaskResult{Error: "region is unavailable", ExitCode: 1}
	}
	return TaskResult{Output: region}
}`,
			acceptance: `func TestFeature001(t *testing.T) {
	result := Feature001(TaskInput{Arguments: []string{"HOME"}}, CapabilityResults{})
	if result.Output == "" || result.Error != "" {
		t.Fatalf("home = %q error = %q", result.Output, result.Error)
	}
}`,
			forbidden: `func Feature001(input TaskInput, dependencies CapabilityResults) TaskResult {
	region, _ := os.LookupEnv("HOME")
	return TaskResult{Output: region}
}`,
			forbiddenName: "LookupEnv",
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			program := goRuntimeCapabilityFixtureProgram(t, []string{fixture.selectedID})
			feature := directCodingTestGeneratedBlockRef(t, program.Source, "feature.001")
			acceptance := directCodingTestGeneratedBlockRef(t, program.Source, "acceptance.001")
			if !reflect.DeepEqual(
				feature.Block.Capabilities, []string{"runtime.api", fixture.selectedID},
			) || !reflect.DeepEqual(
				feature.Block.DependsOn, []string{"runtime.api", fixture.selectedID},
			) {
				t.Fatalf(
					"implementation authority=%q/%q",
					feature.Block.Capabilities, feature.Block.DependsOn,
				)
			}
			if !reflect.DeepEqual(
				acceptance.Block.Capabilities, []string{"runtime.api", "feature.001"},
			) || !reflect.DeepEqual(
				acceptance.Block.DependsOn, []string{"runtime.api", "feature.001"},
			) {
				t.Fatalf(
					"acceptance authority=%q/%q",
					acceptance.Block.Capabilities, acceptance.Block.DependsOn,
				)
			}

			featureInput := assertGoInitialRepairAuthority(t, &program, feature)
			featureVisible := strings.Join(featureInput.Capabilities, "\n")
			if !strings.Contains(featureVisible, fixture.selectedAPI) ||
				strings.Contains(featureVisible, fixture.unselectedAPI) {
				t.Fatalf(
					"selected/unselected implementation projection differs: %q",
					featureVisible,
				)
			}
			for _, hidden := range []string{"os.ReadFile", "os.LookupEnv"} {
				if strings.Contains(featureVisible, hidden) {
					t.Fatalf("model authority leaked wrapper implementation %q", hidden)
				}
			}

			validatedFeature, err := validateDirectCodingGoFragment(
				featureInput, fixture.implementation,
			)
			if err != nil {
				t.Fatal(err)
			}
			program.Generated["feature.001"] = validatedFeature
			acceptanceInput := assertGoInitialRepairAuthority(t, &program, acceptance)
			acceptanceVisible := strings.Join(acceptanceInput.Capabilities, "\n")
			if strings.Contains(acceptanceVisible, "RuntimeReadFile") ||
				strings.Contains(acceptanceVisible, "RuntimeEnvironmentValue") ||
				strings.Contains(acceptanceVisible, "os.ReadFile") ||
				strings.Contains(acceptanceVisible, "os.LookupEnv") {
				t.Fatalf("acceptance received runtime wrapper authority: %q", acceptanceVisible)
			}
			if _, err := validateDirectCodingGoFragment(
				featureInput, fixture.forbidden,
			); err == nil || !strings.Contains(err.Error(), fixture.forbiddenName) {
				t.Fatalf("raw Go package authority error=%v want %q", err, fixture.forbiddenName)
			}
			validatedAcceptance, err := validateDirectCodingGoFragment(
				acceptanceInput, fixture.acceptance,
			)
			if err != nil {
				t.Fatal(err)
			}
			program.Generated["acceptance.001"] = validatedAcceptance
			root := writeGoRuntimeCapabilityFixtureAssembly(t, program)
			if fixture.prepare != nil {
				fixture.prepare(t, root)
			}
			output, err := runDirectCodingStageCommand(
				context.Background(), root, directCodingGoStageTimeout,
				"go", "test", "-count=1", "./...",
			)
			if err != nil {
				t.Fatalf("Go runtime-capability fixture failed: %v\n%s", err, output)
			}
		})
	}
}

func assertGoInitialRepairAuthority(
	t *testing.T,
	program *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) assemblyline.FragmentGenerationInput {
	t.Helper()
	initial, err := directCodingLanguageFragmentInput(program, ref, "go")
	if err != nil {
		t.Fatal(err)
	}
	repairCapabilities, repairSymbols, err := directCodingLanguageRepairContext(program, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(initial.Capabilities, repairCapabilities) ||
		!reflect.DeepEqual(initial.PermittedSymbols, repairSymbols) {
		t.Fatalf(
			"Go %s initial/repair authority differs capabilities=%q/%q globals=%q/%q",
			ref.Block.ID, initial.Capabilities, repairCapabilities,
			initial.PermittedSymbols, repairSymbols,
		)
	}
	return initial
}

func goRuntimeCapabilityFixtureProgram(
	t *testing.T,
	selected []string,
) directCodingProgram {
	t.Helper()
	program := goRuntimeCapabilityBaseProgram(t)
	bound, err := bindDirectCodingGoRuntimeCapabilities(
		program,
		directCodingRuntimeCapabilityGraph{
			"requirement_001": append([]string(nil), selected...),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func goRuntimeCapabilityBaseProgram(t *testing.T) directCodingProgram {
	t.Helper()
	specification, workload := goCommandLineStackFixture(t)
	target := assemblyline.TargetTree{
		StackID: genericGoCommandLineAdapter, VersionProfileID: goCommandLineVersionProfileV1,
		Paths: []string{"feature.go", "feature_test.go"},
	}
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := buildDirectCodingApplicationFileCoveragePlan(
		stack, workload, target,
		map[string][]string{
			workload.Tasks[0].ID: append([]string(nil), target.Paths...),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	program, err := compileDirectCodingProgram(
		"go-runtime-capability-fixture", specification, nil,
		map[string]directCodingSkillBinding{}, workload,
		directCodingCapabilityGraph{"requirement_001": nil}, target, coverage,
	)
	if err != nil {
		t.Fatal(err)
	}
	return program
}

func writeGoRuntimeCapabilityFixtureAssembly(
	t *testing.T,
	program directCodingProgram,
) string {
	t.Helper()
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, file := range assembly.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
