package assemblyline

import (
	"strings"
	"testing"
)

func TestFileContentDecodesOneLeafScope(t *testing.T) {
	input := FileContentInput{Objective: "Build a counter.", Path: "src/counter.tsx", Kind: TargetArtifactImplementation, Requirements: []FileContentRequirement{{ID: "r1", Statement: "display the count"}, {ID: "r2", Statement: "increment the count"}}}
	content, err := DecodeFileContentCandidate(input, `{"schema":"omnidex.file-content.v1","requirement_indexes":[0,1]}`)
	if err != nil {
		t.Fatal(err)
	}
	if content.Path != "src/counter.tsx" || content.Kind != TargetArtifactImplementation || strings.Join(content.RequirementIDs, ",") != "r1,r2" {
		t.Fatalf("content=%+v", content)
	}
}

func TestFileContentCorrectionCarriesExactCandidateAndFailure(t *testing.T) {
	input := FileContentInput{Objective: "Build a counter.", Path: "src/counter.tsx", Kind: TargetArtifactImplementation, Requirements: []FileContentRequirement{{ID: "r1", Statement: "display the count"}}, Correction: &FileContentCorrection{CandidateJSON: `{"schema":"omnidex.file-content.v1","requirement_indexes":[4]}`, Failure: "file-content requirement index 4 is outside the supplied requirement list"}}
	prompt, err := BuildFileContentPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"CURRENT_FILE_CONTENT_CANDIDATE_JSON", "VALIDATION_FAILURE", "requirement index 4"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt misses %q: %s", expected, prompt)
		}
	}
}

func TestFileContentPromptIsBlindToTreeAndPathControl(t *testing.T) {
	input := FileContentInput{
		Objective: "Build a counter.", Path: "src/counter.tsx", Kind: TargetArtifactImplementation,
		Requirements: []FileContentRequirement{{ID: "r1", Statement: "display the count"}},
	}
	prompt, err := BuildFileContentPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"src/counter.tsx", "file_path", "tree", "command", "operation", "implementation", "verification"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("file-content prompt leaks %q: %s", forbidden, prompt)
		}
	}
}
