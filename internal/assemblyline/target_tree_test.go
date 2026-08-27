package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestTargetTreeIsPathOnlyAndOmissionIsUntouched(t *testing.T) {
	input := TargetTreeInput{
		Objective: "Build a counter.", TechnicalContext: "TypeScript React browser project.",
		Constraints:   TargetTreeConstraints{ExactPathCount: 2},
		ExistingPaths: []string{"src/old.ts", "src/counter.ts"}, ReusablePaths: []string{},
		ReservedPaths: []string{"src/runtime.ts"}, ExistingDirs: []string{"src"},
	}
	target, err := DecodeTargetTreeCandidate(input, `{"schema":"omnidex.target-tree.v1","paths":["tests/counter.test.ts","src/counter.ts"]}`)
	if err != nil {
		t.Fatal(err)
	}
	transitions, err := DiffTargetTree(input, target)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) != 3 || transitions[0] != (TargetTreeTransition{Kind: TargetTreeEnsureDirectory, Path: "tests"}) || transitions[1] != (TargetTreeTransition{Kind: TargetTreeReconcile, Path: "src/counter.ts"}) || transitions[2] != (TargetTreeTransition{Kind: TargetTreeCreate, Path: "tests/counter.test.ts"}) {
		t.Fatalf("transitions=%+v", transitions)
	}
}

func TestTargetTreePromptContainsNoContentResponsibility(t *testing.T) {
	prompt, _, err := RenderPortableJob(mustTargetTreeJob(t, TargetTreeInput{
		Objective: "Build a counter.", TechnicalContext: "TypeScript React browser project.",
		Constraints:   TargetTreeConstraints{ExactPathCount: 2},
		ExistingPaths: []string{"src/old.tsx"}, ReusablePaths: []string{"src/shared.tsx"},
		ReservedPaths: []string{"src/App.tsx"}, ExistingDirs: []string{"src"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"requirement", "purpose", "ownership", "declaration", "command", "operation"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("tree prompt leaks %q: %s", forbidden, prompt)
		}
	}
}

func TestTargetTreePromptSeparatesExistingReusableAndReservedPaths(t *testing.T) {
	input := TargetTreeInput{
		Objective: "Build a counter.", TechnicalContext: "TypeScript React browser project.",
		Constraints:   TargetTreeConstraints{ExactPathCount: 2},
		ExistingPaths: []string{"src/existing.tsx"}, ReusablePaths: []string{"src/shared.tsx"},
		ReservedPaths: []string{"src/runtime.tsx"}, ExistingDirs: []string{"src"},
	}
	prompt, err := BuildTargetTreePrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"EXISTING_WORKSPACE_PATHS_JSON:\n[\"src/existing.tsx\"]",
		"REUSABLE_ACCEPTED_PATHS_JSON:\n[\"src/shared.tsx\"]",
		"FORBIDDEN_OUTPUT_PATHS_JSON:\n[\"src/runtime.tsx\"]",
		"Every returned path must be relative to the workspace root and must not start with a slash.",
		"Every returned path and the complete path set must satisfy CODE_SELECTED_TECHNICAL_CONTEXT exactly.",
		"Never return a path listed in FORBIDDEN_OUTPUT_PATHS_JSON",
		"CODE_SELECTED_PATH_CONSTRAINTS_JSON:\n{\"exact_path_count\":2,\"root_files_only\":false}",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("target-tree prompt lacks %q: %s", expected, prompt)
		}
	}
	if strings.Contains(prompt, "CURRENT_OR_RESERVED_PATHS_JSON") {
		t.Fatalf("target-tree prompt retains ambiguous path authority: %s", prompt)
	}
}

func TestTargetTreeRejectsReservedPathEvenWhenItExists(t *testing.T) {
	input := TargetTreeInput{
		Objective: "Build a counter.", TechnicalContext: "TypeScript React browser project.",
		Constraints:   TargetTreeConstraints{ExactPathCount: 2},
		ExistingPaths: []string{"src/runtime.tsx"}, ReusablePaths: []string{},
		ReservedPaths: []string{"src/runtime.tsx"}, ExistingDirs: []string{"src"},
	}
	_, err := DecodeTargetTreeCandidate(
		input,
		`{"schema":"omnidex.target-tree.v1","paths":["src/feature.tsx","src/runtime.tsx"]}`,
	)
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("reserved target-tree path error=%v", err)
	}
}

func TestTargetTreeRejectsFileLeafEqualToExistingDirectory(t *testing.T) {
	input := TargetTreeInput{
		Objective: "Build a formatter.", TechnicalContext: "Root Go package.",
		Constraints:   TargetTreeConstraints{ExactPathCount: 2, RootFilesOnly: true},
		ExistingPaths: []string{}, ReusablePaths: []string{}, ReservedPaths: []string{},
		ExistingDirs: []string{"feature.go"},
	}
	candidate := `{"schema":"omnidex.target-tree.v1","paths":["feature.go","feature_test.go"]}`
	if _, err := DecodeTargetTreeCandidate(input, candidate); err == nil ||
		!strings.Contains(err.Error(), "existing workspace directory") {
		t.Fatalf("existing-directory candidate error=%v", err)
	}
	if _, err := DiffTargetTree(
		input, TargetTree{Paths: []string{"feature.go", "feature_test.go"}},
	); err == nil || !strings.Contains(err.Error(), "existing workspace directory") {
		t.Fatalf("existing-directory transition error=%v", err)
	}
}

func TestTargetTreeDiffUsesOnlyFilesystemPathsForReconciliation(t *testing.T) {
	input := TargetTreeInput{
		Objective: "Build a counter.", TechnicalContext: "TypeScript React browser project.",
		Constraints:   TargetTreeConstraints{ExactPathCount: 3},
		ExistingPaths: []string{"existing.ts"}, ReusablePaths: []string{"shared.ts"},
		ReservedPaths: []string{"runtime.ts"}, ExistingDirs: []string{},
	}
	target, err := DecodeTargetTreeCandidate(
		input,
		`{"schema":"omnidex.target-tree.v1","paths":["existing.ts","shared.ts","new.ts"]}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	transitions, err := DiffTargetTree(input, target)
	if err != nil {
		t.Fatal(err)
	}
	want := []TargetTreeTransition{
		{Kind: TargetTreeReconcile, Path: "existing.ts"},
		{Kind: TargetTreeCreate, Path: "new.ts"},
		{Kind: TargetTreeCreate, Path: "shared.ts"},
	}
	if !reflect.DeepEqual(transitions, want) {
		t.Fatalf("transitions=%+v want=%+v", transitions, want)
	}
}

func TestTargetTreeInputRejectsMissingPathAuthority(t *testing.T) {
	valid := TargetTreeInput{
		Objective: "Build a counter.", TechnicalContext: "TypeScript React browser project.",
		Constraints:   TargetTreeConstraints{ExactPathCount: 2},
		ExistingPaths: []string{}, ReusablePaths: []string{}, ReservedPaths: []string{},
		ExistingDirs: []string{},
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*TargetTreeInput)
		want   string
	}{
		{name: "existing", mutate: func(input *TargetTreeInput) { input.ExistingPaths = nil }, want: "existing workspace paths"},
		{name: "reusable", mutate: func(input *TargetTreeInput) { input.ReusablePaths = nil }, want: "reusable accepted paths"},
		{name: "reserved", mutate: func(input *TargetTreeInput) { input.ReservedPaths = nil }, want: "reserved paths"},
		{name: "directories", mutate: func(input *TargetTreeInput) { input.ExistingDirs = nil }, want: "existing workspace directories"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := valid
			testCase.mutate(&input)
			err := input.Validate()
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("missing %s authority error=%v", testCase.name, err)
			}
		})
	}
}

func TestTargetTreeConstraintsOwnCardinalityAndRootLocation(t *testing.T) {
	input := TargetTreeInput{
		Objective: "Build a formatter.", TechnicalContext: "Root JavaScript modules.",
		Constraints:   TargetTreeConstraints{ExactPathCount: 2, RootFilesOnly: true},
		ExistingPaths: []string{}, ReusablePaths: []string{}, ReservedPaths: []string{},
		ExistingDirs: []string{},
	}
	for name, candidate := range map[string]string{
		"too few": `{"schema":"omnidex.target-tree.v1","paths":["format.mjs"]}`,
		"nested":  `{"schema":"omnidex.target-tree.v1","paths":["src/format.mjs","format.test.mjs"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeTargetTreeCandidate(input, candidate); err == nil {
				t.Fatalf("target-tree constraints accepted %s", candidate)
			}
		})
	}
	schema, err := TargetTreeResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	paths := properties["paths"].(map[string]any)
	items := paths["items"].(map[string]any)
	if paths["minItems"] != 2 || paths["maxItems"] != 2 || items["pattern"] != `^[^/]{1,512}$` {
		t.Fatalf("target-tree response schema lacks exact constraints: %+v", paths)
	}
}

func mustTargetTreeJob(t *testing.T, input TargetTreeInput) PortableJob {
	t.Helper()
	job, err := NewTargetTreeJob(input)
	if err != nil {
		t.Fatal(err)
	}
	return job
}
