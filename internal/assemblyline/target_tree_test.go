package assemblyline

import (
	"strings"
	"testing"
)

func TestTargetTreeIsPathOnlyAndOmissionIsUntouched(t *testing.T) {
	input := TargetTreeInput{Objective: "Build a counter.", TechnicalContext: "TypeScript React browser project.", ExistingPaths: []string{"src/old.ts", "src/counter.ts"}, ExistingDirs: []string{"src"}}
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
	prompt, _, err := RenderPortableJob(mustTargetTreeJob(t, TargetTreeInput{Objective: "Build a counter.", TechnicalContext: "TypeScript React browser project.", ExistingPaths: []string{"src/App.tsx"}, ExistingDirs: []string{"src"}}))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"requirement", "purpose", "ownership", "declaration", "command", "operation"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("tree prompt leaks %q: %s", forbidden, prompt)
		}
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
