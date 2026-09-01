package worker

import (
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptBrowserCompilerOwnsIndependentVerificationLeaves(t *testing.T) {
	fixtures := []struct {
		name        string
		product     string
		requirement string
	}{
		{
			name:        "maintenance tracker",
			product:     "A maintenance tracker",
			requirement: "Record one scheduled maintenance task and expose its current status.",
		},
		{
			name:        "text summarizer",
			product:     "A text summarizer",
			requirement: "Accept supplied text and expose its resulting summary.",
		},
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			specification := assemblyline.ApplicationSpecification{
				Surface:      assemblyline.ApplicationSurfaceBrowser,
				ProductQuote: fixture.product,
				Requirements: []assemblyline.Requirement{{
					ID: "requirement_001", SourceQuote: fixture.requirement,
				}},
			}
			workload, err := assemblyline.FreezeApplicationWorkload(specification)
			if err != nil {
				t.Fatalf("freeze fixture workload: %v", err)
			}
			stack, profile := testTypeScriptBrowserProject(t)
			target, coverage, err := resolveDirectCodingTargetTree(
				specification, workload, stack, nil, directCodingTargetTreeOccupation{},
			)
			if err != nil {
				t.Fatalf("resolve browser target tree: %v", err)
			}
			if len(target.Paths) != 2 {
				t.Fatalf("target paths=%v; want one neutral implementation/verification pair", target.Paths)
			}
			files, err := coverage.FilesForTask(workload.Tasks[0].ID)
			if err != nil {
				t.Fatalf("read task coverage: %v", err)
			}
			kinds := make(map[assemblyline.TargetArtifactKind]int)
			for _, file := range files {
				kinds[file.Kind]++
			}
			if kinds[assemblyline.TargetArtifactImplementation] != 1 ||
				kinds[assemblyline.TargetArtifactVerification] != 1 {
				t.Fatalf("coverage kinds=%v; want exactly one implementation and verification", kinds)
			}

			blueprint, staticFiles, err := compileGenericTypeScriptBrowserBlueprint(
				fixture.name, specification, workload,
				directCodingCapabilityGraph{"requirement_001": nil},
				profile, target, coverage,
			)
			if err != nil {
				t.Fatalf("compile browser blueprint: %v", err)
			}
			if err := validateDirectCodingSinglePairSourceOwnership(workload, blueprint); err != nil {
				t.Fatalf("validate generated source pair: %v", err)
			}
			implementationID, err := directCodingTaskBlockIDByRole(
				blueprint, workload.Tasks[0].ID, assemblyline.SourceBlockTaskImplementation,
			)
			if err != nil {
				t.Fatalf("implementation block: %v", err)
			}
			verificationID, err := directCodingTaskBlockIDByRole(
				blueprint, workload.Tasks[0].ID, assemblyline.SourceBlockTaskVerification,
			)
			if err != nil {
				t.Fatalf("verification block: %v", err)
			}
			verification, exists := directCodingSourceBlueprintBlock(blueprint, verificationID)
			if !exists || !testBlockDependsOn(verification, implementationID) {
				t.Fatalf("verification %q does not directly depend on implementation %q", verificationID, implementationID)
			}
			manifest := testBrowserManifest(t, staticFiles)
			for _, script := range []string{"test", "typecheck", "build"} {
				if manifest.Scripts[script] == "" {
					t.Fatalf("browser manifest omits deterministic %s command", script)
				}
			}
			for _, dependency := range []string{
				"@testing-library/jest-dom",
				"@testing-library/react",
				"dom-accessibility-api",
				"jsdom",
				"vitest",
			} {
				if manifest.DevDependencies[dependency] == "" {
					t.Fatalf("browser manifest omits direct verifier dependency %s", dependency)
				}
			}
		})
	}
}
