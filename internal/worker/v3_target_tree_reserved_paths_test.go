package worker

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTargetTreeInputProjectsOneCurrentAndReservedTree(t *testing.T) {
	browserSpecification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceBrowser,
		"path authority", "preserve one exact path boundary", "add another behavior",
	)
	typeScriptStack, err := directCodingProjectStackByID(genericTypeScriptBrowserAdapter)
	if err != nil {
		t.Fatal(err)
	}
	input, err := directCodingTargetTreeInput(
		"path authority request", browserSpecification, workload, typeScriptStack,
		[]string{"src/App.tsx", "src/existing.tsx"},
		[]string{"src"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(
		input.ExistingPaths, []string{"src/existing.tsx"},
	) || !reflect.DeepEqual(input.ReservedPaths, []string{
		"src/App.test.tsx", "src/App.tsx", "src/main.tsx",
		"src/runtime.test.tsx", "src/runtime.tsx",
	}) || input.Objective != "path authority request" {
		t.Fatalf("complete path projection=%+v", input)
	}
}

func TestMechanicallyProjectedStackGetsNonNilPathAuthority(t *testing.T) {
	specification, workload := testApplicationFileCoverageAuthority(
		t, assemblyline.ApplicationSurfaceService,
		"record service", "return one record",
	)
	stack, err := directCodingProjectStackByID(genericPHPServiceAdapter)
	if err != nil {
		t.Fatal(err)
	}
	input, err := directCodingTargetTreeInput(
		"record service request", specification, workload, stack, []string{}, []string{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if input.ExistingPaths == nil || input.ReservedPaths == nil ||
		input.ExistingDirs == nil {
		t.Fatalf("mechanical stack received nil path authority: %+v", input)
	}
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteProjectedStacksRegisterNormalizedCodeOwnedTargetPaths(t *testing.T) {
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
		if stack.ProjectCompleteTargetTree == nil {
			continue
		}
		expected, exists := want[stack.ID]
		if !exists {
			t.Fatalf("complete-projected stack %s lacks an expected reservation contract", stack.ID)
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
		t.Fatalf("complete-projected stack reservation coverage=%d want=%d", len(seen), len(want))
	}
}

func TestEveryRegisteredStackHasOneMechanicalTargetTreeProjector(t *testing.T) {
	for _, stack := range registeredDirectCodingProjectStacks() {
		complete := stack.ProjectCompleteTargetTree != nil
		focused := stack.ProjectFocusedTargetTree != nil
		if complete == focused {
			t.Fatalf(
				"stack %s complete/focused target-tree projectors=%t/%t want exactly one",
				stack.ID, complete, focused,
			)
		}
	}
}

func TestTargetTreeRegistryRejectsCompetingMechanicalProjectors(t *testing.T) {
	stacks := registeredDirectCodingProjectStacks()
	for index := range stacks {
		if stacks[index].ID == genericTypeScriptBrowserAdapter {
			stacks[index].ProjectFocusedTargetTree = projectGoCommandLineFocusedTargetTree
		}
	}
	err := validateDirectCodingArtifactRegistriesFrom(
		registeredDirectCodingArtifactAdapters(),
		registeredDirectCodingProjectVersionProfiles(),
		stacks,
		registeredDirectCodingParserQualifications(),
	)
	if err == nil || !strings.Contains(err.Error(), "both complete and focused") {
		t.Fatalf("competing target-tree projector error=%v", err)
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
