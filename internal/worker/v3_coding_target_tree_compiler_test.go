package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestProgramCompilerValidatesSelectedStackTargetTreeBeforeCoverage(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		stackID string
		surface assemblyline.ApplicationSurface
		paths   []string
	}{
		{
			name: "TypeScript reserved leaves", stackID: genericTypeScriptBrowserAdapter,
			surface: assemblyline.ApplicationSurfaceBrowser,
			paths:   []string{"src/App.tsx", "src/App.test.tsx"},
		},
		{
			name: "Go nested leaves", stackID: genericGoCommandLineAdapter,
			surface: assemblyline.ApplicationSurfaceCommandLine,
			paths:   []string{"nested/feature.go", "nested/feature_test.go"},
		},
		{
			name: "JavaScript nested leaves", stackID: genericJavaScriptCommandLineAdapter,
			surface: assemblyline.ApplicationSurfaceCommandLine,
			paths:   []string{"nested/feature.mjs", "nested/feature.test.mjs"},
		},
		{
			name: "Rust mismatched leaves", stackID: genericRustCommandLineAdapter,
			surface: assemblyline.ApplicationSurfaceCommandLine,
			paths:   []string{"src/alpha.rs", "tests/beta_test.rs"},
		},
		{
			name: "Java reserved leaves", stackID: genericJavaCommandLineAdapter,
			surface: assemblyline.ApplicationSurfaceCommandLine,
			paths:   []string{"Main.java", "MainTest.java"},
		},
		{
			name: "PHP mismatched leaves", stackID: genericPHPServiceAdapter,
			surface: assemblyline.ApplicationSurfaceService,
			paths:   []string{"src/Feature001.php", "tests/Feature002Test.php"},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			specification, workload := targetTreeCompilerFixture(t, testCase.surface)
			_, err := compileDirectCodingProgram(
				"compiler-boundary", specification, nil, nil, workload,
				directCodingCapabilityGraph{"requirement_001": nil},
				assemblyline.TargetTree{
					StackID:          testCase.stackID,
					VersionProfileID: testVersionProfileIDForStack(t, testCase.stackID),
					Paths:            testCase.paths,
				},
				assemblyline.ApplicationFileCoveragePlan{},
			)
			if err == nil {
				t.Fatal("compiler accepted a stack-invalid target tree")
			}
			if !strings.Contains(err.Error(), "validate "+testCase.stackID+" target tree") {
				t.Fatalf("error=%v", err)
			}
			if strings.Contains(err.Error(), "coverage") {
				t.Fatalf("target-tree validation ran after coverage: %v", err)
			}
		})
	}
}

func targetTreeCompilerFixture(
	t *testing.T,
	surface assemblyline.ApplicationSurface,
) (assemblyline.ApplicationSpecification, assemblyline.FrozenApplicationWorkload) {
	t.Helper()
	specification := assemblyline.ApplicationSpecification{
		Surface: surface, ProductQuote: "one bounded application behavior",
		Requirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Produce one observable result",
		}},
	}
	if err := specification.Validate(); err != nil {
		t.Fatal(err)
	}
	workload, err := assemblyline.FreezeApplicationWorkload(specification)
	if err != nil {
		t.Fatal(err)
	}
	return specification, workload
}
