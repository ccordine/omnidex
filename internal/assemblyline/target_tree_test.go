package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestRawTargetTreeParserBuildsOnlyNormalizedFilePaths(t *testing.T) {
	t.Parallel()
	target, err := ParseTargetTree(strings.Join([]string{
		"ROOT",
		"  D tests",
		"    F counter.test.tsx",
		"  D src",
		"    D components",
		"      F counter.tsx",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"src/components/counter.tsx", "tests/counter.test.tsx"}
	if !reflect.DeepEqual(target.Paths, want) {
		t.Fatalf("paths=%v want=%v", target.Paths, want)
	}
}

func TestRawTargetTreeParserRejectsNonTreeAndUnsafeShapes(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"json":           "{\"paths\":[\"src/app.tsx\"]}",
		"flat path":      "ROOT\nsrc/app.tsx",
		"blank line":     "ROOT\n\n  F app.tsx",
		"fence":          "~~~text\nROOT\n~~~",
		"prose":          "Here is the tree:\nROOT",
		"slash name":     "ROOT\n  F src/app.tsx",
		"backslash":      "ROOT\n  F src\\app.tsx",
		"traversal":      "ROOT\n  D ..\n    F app.tsx",
		"absolute":       "ROOT\n  F /tmp",
		"drive":          "ROOT\n  F C:",
		"drive relative": "ROOT\n  F C:secret.tsx",
		"carriage":       "ROOT\r\n  F app.tsx",
		"duplicate":      "ROOT\n  F app.tsx\n  F app.tsx",
		"collision":      "ROOT\n  D app\n    F child.ts\n  F app",
		"file children":  "ROOT\n  F app.tsx\n    F child.tsx",
		"empty dir":      "ROOT\n  D src",
		"odd indent":     "ROOT\n F app.tsx",
		"depth skip":     "ROOT\n    F app.tsx",
	}
	for name, raw := range tests {
		name, raw := name, raw
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ParseTargetTree(raw); err == nil {
				t.Fatalf("unsafe raw tree was accepted: %q", raw)
			}
		})
	}
}

func TestTargetTreeDepthLimitIsSharedByRawAndPathValidation(t *testing.T) {
	t.Parallel()
	parts := make([]string, MaxTargetTreeDepth+1)
	lines := []string{targetTreeRootLine}
	for index := range parts {
		parts[index] = "d"
		kind := "D"
		if index == len(parts)-1 {
			kind = "F"
		}
		lines = append(lines, strings.Repeat("  ", index+1)+kind+" d")
	}
	if _, err := ParseTargetTree(strings.Join(lines, "\n")); err == nil {
		t.Fatalf("raw tree deeper than %d was accepted", MaxTargetTreeDepth)
	}
	if _, err := RenderTargetTree([]string{strings.Join(parts, "/")}); err == nil {
		t.Fatalf("path deeper than %d was rendered", MaxTargetTreeDepth)
	}
}

func TestRenderTargetTreeIsCanonical(t *testing.T) {
	t.Parallel()
	rendered, err := RenderTargetTree([]string{
		"z.txt", "src/z.ts", "src/a.ts", "tests/unit/a.test.ts",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"ROOT",
		"  D src",
		"    F a.ts",
		"    F z.ts",
		"  D tests",
		"    D unit",
		"      F a.test.ts",
		"  F z.txt",
	}, "\n")
	if rendered != want {
		t.Fatalf("rendered tree:\n%s\nwant:\n%s", rendered, want)
	}
	if _, err := RenderTargetTree([]string{"src", "src/app.tsx"}); err == nil {
		t.Fatal("file/directory collision was accepted")
	}
}

func TestTargetTreePromptUsesRawCurrentAndReservedTrees(t *testing.T) {
	t.Parallel()
	input := targetTreeTestInput()
	input.ExistingPaths = []string{"src/existing.tsx"}
	input.ReservedPaths = []string{"src/App.tsx", "src/runtime.tsx"}
	prompt, err := RenderPortableJob(mustTargetTreeJob(t, input))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"ACCEPTED_GOALS:\nProduct context: counter\nAccepted goal 1: display a count",
		"CURRENT_MANAGED_WORKLOAD_TREE:\nROOT\n  D src\n    F existing.tsx",
		"RESERVED_TREE:\nROOT\n  D src\n    F App.tsx\n    F runtime.tsx",
		"RAW_TREE_GRAMMAR:\nROOT\n  D <single basename>",
		"complete expected workload tree, not a delta",
	} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("target-tree prompt lacks %q:\n%s", expected, prompt)
		}
	}
	if strings.Contains(prompt, "_JSON") {
		t.Fatalf("raw target-tree renderer returned JSON authority: prompt=%s", prompt)
	}
	for _, forbidden := range []string{
		"artifact metadata", "filesystem operations", "file contents", "source",
		"declarations", "commands", "ownership", "dependencies", "completion state",
		"code-selected", "CODE_SELECTED", "code-reserved", "CODE_RESERVED",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("target-tree prompt contains unrelated responsibility %q: %s", forbidden, prompt)
		}
	}
}

func TestTargetTreeDecodeAppliesCodeSelectedConstraints(t *testing.T) {
	t.Parallel()
	input := targetTreeTestInput()
	input.ReservedPaths = []string{"src/runtime.tsx"}
	input.ExistingDirs = []string{"tests/counter.test.tsx"}
	for name, raw := range map[string]string{
		"too few":   "ROOT\n  D src\n    F counter.tsx",
		"reserved":  "ROOT\n  D src\n    F counter.tsx\n    F runtime.tsx",
		"directory": "ROOT\n  D src\n    F counter.tsx\n  D tests\n    F counter.test.tsx",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeTargetTreeCandidate(input, raw); err == nil {
				t.Fatalf("invalid candidate accepted: %s", raw)
			}
		})
	}
	rootOnly := input
	rootOnly.Constraints = TargetTreeConstraints{ExactPathCount: 2, RootFilesOnly: true}
	if _, err := DecodeTargetTreeCandidate(
		rootOnly, "ROOT\n  D src\n    F counter.tsx\n  F counter.test.tsx",
	); err == nil {
		t.Fatal("nested tree passed root-files-only constraint")
	}
}

func TestTargetTreeDecodeRejectsReservedFileHierarchyCrossing(t *testing.T) {
	t.Parallel()
	input := targetTreeTestInput()
	input.ReservedPaths = []string{"src/App.tsx"}
	_, err := DecodeTargetTreeCandidate(input, strings.Join([]string{
		"ROOT",
		"  D src",
		"    D App.tsx",
		"      F child.tsx",
		"  D tests",
		"    F child.test.tsx",
	}, "\n"))
	if err == nil || !strings.Contains(err.Error(), "crosses reserved file boundary") {
		t.Fatalf("reserved ancestor error=%v", err)
	}
}

func TestTargetTreeDiffRequiresExplicitDeletionEligibility(t *testing.T) {
	t.Parallel()
	input := targetTreeTestInput()
	input.ExistingPaths = []string{"src/counter.tsx", "src/old.tsx"}
	input.ExistingDirs = []string{"src"}
	target, err := DecodeTargetTreeCandidate(input, strings.Join([]string{
		"ROOT",
		"  D src",
		"    F counter.tsx",
		"  D tests",
		"    F counter.test.tsx",
	}, "\n"))
	if err != nil {
		t.Fatal(err)
	}
	withoutDelete, err := DiffTargetTree(input, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, transition := range withoutDelete {
		if transition.Kind == TargetTreeDelete {
			t.Fatalf("unscoped omission produced deletion: %+v", withoutDelete)
		}
	}
	withDelete, err := DiffTargetTree(input, target, []string{"src/old.tsx"})
	if err != nil {
		t.Fatal(err)
	}
	want := []TargetTreeTransition{
		{Kind: TargetTreeDelete, Path: "src/old.tsx"},
		{Kind: TargetTreeEnsureDirectory, Path: "tests"},
		{Kind: TargetTreeReconcile, Path: "src/counter.tsx"},
		{Kind: TargetTreeCreate, Path: "tests/counter.test.tsx"},
	}
	if !reflect.DeepEqual(withDelete, want) {
		t.Fatalf("transitions=%+v want=%+v", withDelete, want)
	}
	if _, err := DiffTargetTree(input, target, []string{"src/missing.tsx"}); err == nil {
		t.Fatal("non-current deletion eligibility was accepted")
	}
}

func TestTargetTreeInputRequiresCompletePathAuthority(t *testing.T) {
	t.Parallel()
	valid := targetTreeTestInput()
	for _, testCase := range []struct {
		name   string
		mutate func(*TargetTreeInput)
	}{
		{name: "existing", mutate: func(input *TargetTreeInput) { input.ExistingPaths = nil }},
		{name: "reserved", mutate: func(input *TargetTreeInput) { input.ReservedPaths = nil }},
		{name: "directories", mutate: func(input *TargetTreeInput) { input.ExistingDirs = nil }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			input := valid
			testCase.mutate(&input)
			if err := input.Validate(); err == nil {
				t.Fatalf("missing %s authority was accepted", testCase.name)
			}
		})
	}
	pathBearing := valid
	pathBearing.Objective = "Read /private/workspace/secret.tsx."
	if err := pathBearing.Validate(); err == nil || !strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("absolute accepted-goal identity error=%v", err)
	}
	pathBearing = valid
	pathBearing.TechnicalContext = "Place one leaf under src/private."
	if err := pathBearing.Validate(); err == nil || !strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("technical-context path identity error=%v", err)
	}
	pathBearing = valid
	pathBearing.Correction = &TargetTreeCorrection{
		CandidateTree: "ROOT\n  D src\n    F counter.tsx\n  D tests\n    F counter.test.tsx",
		Failure:       "The node at src/counter.tsx is invalid.",
	}
	if err := pathBearing.Validate(); err == nil || !strings.Contains(err.Error(), "filesystem identity") {
		t.Fatalf("correction path identity error=%v", err)
	}
}

func targetTreeTestInput() TargetTreeInput {
	return TargetTreeInput{
		Objective:        "Product context: counter\nAccepted goal 1: display a count",
		TechnicalContext: "TypeScript React browser project.",
		Constraints:      TargetTreeConstraints{ExactPathCount: 2},
		ExistingPaths:    []string{}, ReservedPaths: []string{}, ExistingDirs: []string{},
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
