package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestFileContentResolutionUsesOneLeafAndExactCorrection(t *testing.T) {
	calls := 0
	runtime := typedWorkerRuntime{Context: context.Background(), MaxAttempts: 2, Execute: testPortableExecutor(func(_ string, model, prompt string, _ map[string]any) (string, error) {
		calls++
		switch calls {
		case 1:
			if model != "content-model" || !strings.Contains(prompt, "ACCEPTED_REQUIREMENTS_JSON") {
				t.Fatalf("initial dispatch model=%q prompt=%s", model, prompt)
			}
			return `{"schema":"omnidex.file-content.v1","requirement_indexes":[3]}`, nil
		case 2:
			if model != "correction-model" {
				t.Fatalf("correction model=%q", model)
			}
			for _, expected := range []string{"CURRENT_FILE_CONTENT_CANDIDATE_JSON", "VALIDATION_FAILURE", "index 3"} {
				if !strings.Contains(prompt, expected) {
					t.Fatalf("correction prompt missing %q: %s", expected, prompt)
				}
			}
			return `{"schema":"omnidex.file-content.v1","requirement_indexes":[0]}`, nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return "", nil
		}
	})}
	content, err := resolveDirectCodingFileContent(runtime, "content-model", "correction-model", assemblyline.FileContentInput{
		Objective: "Build a counter.", Path: "src/counter.tsx", Kind: assemblyline.TargetArtifactImplementation, Requirements: []assemblyline.FileContentRequirement{{ID: "requirement_001", Statement: "display a count"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || content.Kind != assemblyline.TargetArtifactImplementation || strings.Join(content.RequirementIDs, ",") != "requirement_001" {
		t.Fatalf("calls=%d content=%+v", calls, content)
	}
}

func TestFileContentResolutionDerivesForcedSinglePairWithoutModelCall(t *testing.T) {
	specification := assemblyline.ApplicationSpecification{Requirements: []assemblyline.Requirement{
		{ID: "requirement_001", SourceQuote: "display a count"},
		{ID: "requirement_002", SourceQuote: "increment the count"},
	}}
	tree, err := resolveDirectCodingFileContents(
		typedWorkerRuntime{}, "", "", "", specification,
		assemblyline.TargetTree{
			StackID: genericTypeScriptBrowserAdapter,
			Paths:   []string{"src/Counter.tsx", "tests/Counter.test.tsx"},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Contents) != 2 {
		t.Fatalf("contents=%+v", tree.Contents)
	}
	for _, requirement := range specification.Requirements {
		files, err := tree.RequirementFiles(requirement.ID)
		if err != nil {
			t.Fatal(err)
		}
		if files.ImplementationPath != "src/Counter.tsx" || files.VerificationPath != "tests/Counter.test.tsx" {
			t.Fatalf("requirement %q files=%+v", requirement.ID, files)
		}
	}
}
