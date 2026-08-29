package worker

import (
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestDesiredStateProductProviderReturnsOnlyCurrentRawCollectionLeaves(t *testing.T) {
	t.Parallel()
	const request = "An independently owned Go artifact declaring func Added() int that returns 2 must exist."
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceExisting,
	)
	if err != nil {
		t.Fatal(err)
	}

	contextPrompt, err := assemblyline.BuildApplicationContextNeedCoveragePrompt(
		assemblyline.ApplicationContextNeedLeafInput{
			UserRequest: request, Context: context, AcceptedQuestions: []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	authority := assemblyline.RepositoryRequirementInterpretationInput{
		UserRequest: request, Context: context,
	}
	empty := assemblyline.RepositoryRequirementLeafInput{
		Authority: authority, AcceptedRequirements: []string{},
	}
	coveragePrompt, err := assemblyline.BuildRepositoryRequirementCoveragePrompt(empty)
	if err != nil {
		t.Fatal(err)
	}
	requirementPrompt, err := assemblyline.BuildRepositoryRequirementPrompt(empty)
	if err != nil {
		t.Fatal(err)
	}
	retained := empty
	retained.AcceptedRequirements = []string{request}
	completePrompt, err := assemblyline.BuildRepositoryRequirementCoveragePrompt(retained)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, prompt, response string
		kind                   assemblyline.WorkKind
	}{
		{
			name: "context coverage", prompt: contextPrompt,
			response: assemblyline.ApplicationNoUncoveredContextNeed,
			kind:     assemblyline.WorkApplicationContextNeedCoverage,
		},
		{
			name: "requirement remains", prompt: coveragePrompt,
			response: assemblyline.RepositoryRequirementRemains,
			kind:     assemblyline.WorkRepositoryRequirementCoverage,
		},
		{
			name: "one requirement", prompt: requirementPrompt,
			response: request, kind: assemblyline.WorkRepositoryRequirement,
		},
		{
			name: "requirement complete", prompt: completePrompt,
			response: assemblyline.RepositoryNoUncoveredRequirement,
			kind:     assemblyline.WorkRepositoryRequirementCoverage,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, kind, err := desiredStateProductResponse(
				llm.PreparedModel{Prompt: test.prompt},
			)
			if err != nil {
				t.Fatal(err)
			}
			if response != test.response || kind != test.kind {
				t.Fatalf("response=%q kind=%q want %q/%q", response, kind, test.response, test.kind)
			}
			if json.Valid([]byte(response)) {
				t.Fatalf("raw semantic response became JSON: %q", response)
			}
		})
	}
}

func TestDesiredStateProductProviderRejectsRetiredRequirementEnvelope(t *testing.T) {
	t.Parallel()
	if _, err := desiredStateProductRequirementSource(
		"CURRENT_REQUEST:\nlegacy aggregate authority",
	); err == nil {
		t.Fatal("retired aggregate requirement envelope was accepted")
	}
}
