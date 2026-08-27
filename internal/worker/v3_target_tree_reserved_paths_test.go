package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTargetTreeInputProjectsShareableAndExclusiveEarlierPaths(t *testing.T) {
	browserSpecification, _ := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"path authority", "preserve one exact path boundary",
	)
	typeScriptStack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	shareable, err := directCodingTargetTreeInput(
		browserSpecification, typeScriptStack,
		[]string{"src/App.tsx", "src/existing.tsx"},
		[]string{"src/feature.tsx"}, []string{"src"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(
		shareable.ExistingPaths, []string{"src/App.tsx", "src/existing.tsx"},
	) || !reflect.DeepEqual(shareable.ReusablePaths, []string{"src/feature.tsx"}) ||
		!reflect.DeepEqual(shareable.ReservedPaths, []string{
			"src/App.test.tsx", "src/App.tsx", "src/main.tsx",
			"src/runtime.test.tsx", "src/runtime.tsx",
		}) {
		t.Fatalf("shareable path projection=%+v", shareable)
	}

	commandSpecification, _ := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceCommandLine,
		"path authority", "preserve one exact path boundary",
	)
	rustStack, err := directCodingProjectStackByID(genericRustCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	exclusive, err := directCodingTargetTreeInput(
		commandSpecification, rustStack, []string{"src/existing.rs"},
		[]string{"src/feature.rs", "tests/feature_test.rs"}, []string{"src", "tests"},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantReserved := []string{
		"src/feature.rs", "src/lib.rs", "src/main.rs", "src/runtime.rs",
		"tests/feature_test.rs",
	}
	if len(exclusive.ReusablePaths) != 0 || !reflect.DeepEqual(exclusive.ReservedPaths, wantReserved) {
		t.Fatalf("exclusive path projection=%+v want reserved=%v", exclusive, wantReserved)
	}
}

func TestMechanicallyProjectedStackGetsNonNilPathAuthority(t *testing.T) {
	specification, _ := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceService,
		"record service", "return one record",
	)
	stack, err := directCodingProjectStackByID(genericPHPServiceAdapter)
	if err != nil {
		t.Fatal(err)
	}
	input, err := directCodingTargetTreeInput(
		specification, stack, []string{}, nil, []string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if input.ExistingPaths == nil || input.ReusablePaths == nil || input.ReservedPaths == nil ||
		input.ExistingDirs == nil {
		t.Fatalf("mechanical stack received nil path authority: %+v", input)
	}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestInferredProjectStacksRegisterNormalizedCodeOwnedTargetPaths(t *testing.T) {
	want := map[string][]string{
		genericTypeScriptBrowserAdapter: {
			"src/App.test.tsx", "src/App.tsx", "src/main.tsx",
			"src/runtime.test.tsx", "src/runtime.tsx",
		},
	}
	wantConstraints := map[string]assemblyline.TargetTreeConstraints{
		genericTypeScriptBrowserAdapter: {ExactPathCount: 2},
	}
	seen := make(map[string]struct{}, len(want))
	for _, stack := range registeredDirectCodingProjectStacks() {
		if stack.ProjectFocusedTargetTree != nil {
			continue
		}
		expected, exists := want[stack.ID]
		if !exists {
			t.Fatalf("inferred project stack %s lacks an expected reservation contract", stack.ID)
		}
		if !reflect.DeepEqual(stack.TargetTreeReservedPaths, expected) {
			t.Fatalf("stack %s reservations=%v want=%v", stack.ID, stack.TargetTreeReservedPaths, expected)
		}
		if stack.TargetTreeConstraints != wantConstraints[stack.ID] {
			t.Fatalf(
				"stack %s constraints=%+v want=%+v",
				stack.ID, stack.TargetTreeConstraints, wantConstraints[stack.ID],
			)
		}
		for _, artifactPath := range stack.TargetTreeReservedPaths {
			normalized, err := normalizeDirectCodingPath(artifactPath)
			if err != nil || normalized != artifactPath {
				t.Fatalf("stack %s has invalid reserved path %q: %v", stack.ID, artifactPath, err)
			}
			if _, _, err := directCodingArtifactAdapterForTreePath(stack, artifactPath); err != nil {
				t.Fatalf("stack %s reserved path %q is not model-addressable: %v", stack.ID, artifactPath, err)
			}
		}
		seen[stack.ID] = struct{}{}
	}
	if len(seen) != len(want) {
		t.Fatalf("inferred stack reservation coverage=%d want=%d", len(seen), len(want))
	}
}

func TestTargetTreeRegistryRejectsInvalidConstraints(t *testing.T) {
	stacks := registeredDirectCodingProjectStacks()
	for index := range stacks {
		if stacks[index].ID == genericGoCommandLineAdapter {
			stacks[index].TargetTreeConstraints.ExactPathCount = 0
		}
	}
	err := validateDirectCodingArtifactRegistriesFrom(
		registeredDirectCodingArtifactAdapters(),
		registeredDirectCodingProjectVersionProfiles(),
		stacks,
		registeredDirectCodingParserQualifications(),
	)
	if err == nil || !strings.Contains(err.Error(), "exact path count") {
		t.Fatalf("invalid target-tree constraints error=%v", err)
	}
}

func TestSinglePairValidatorConsumesStackReservationAuthority(t *testing.T) {
	stack, err := directCodingProjectStackByID(genericGoCommandLineAdapter)
	if err != nil {
		t.Fatal(err)
	}
	stack.TargetTreeReservedPaths = []string{"feature.go"}
	target := assemblyline.TargetTree{
		StackID: stack.ID, Paths: []string{"feature.go", "feature_test.go"},
	}
	if err := validateDirectCodingFocusedTargetTree(stack, target); err == nil ||
		!strings.Contains(err.Error(), "reserved") {
		t.Fatalf("stack reservation was not authoritative: %v", err)
	}
}

func TestTargetTreeReservationRegistryRejectsInvalidPaths(t *testing.T) {
	tests := []struct {
		name        string
		paths       []string
		wantFailure string
	}{
		{name: "unnormalized", paths: []string{"./main.go"}, wantFailure: "not normalized"},
		{name: "unordered", paths: []string{"runtime.go", "main.go"}, wantFailure: "duplicated or unordered"},
		{name: "duplicate", paths: []string{"main.go", "main.go"}, wantFailure: "duplicated or unordered"},
		{name: "outside target adapters", paths: []string{"go.mod"}, wantFailure: "matches 0 target adapters"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stacks := registeredDirectCodingProjectStacks()
			for index := range stacks {
				if stacks[index].ID == genericGoCommandLineAdapter {
					stacks[index].TargetTreeReservedPaths = test.paths
				}
			}
			err := validateDirectCodingArtifactRegistriesFrom(
				registeredDirectCodingArtifactAdapters(),
				registeredDirectCodingProjectVersionProfiles(),
				stacks,
				registeredDirectCodingParserQualifications(),
			)
			if err == nil || !strings.Contains(err.Error(), test.wantFailure) {
				t.Fatalf("invalid reservation error=%v want fragment=%q", err, test.wantFailure)
			}
		})
	}
}
