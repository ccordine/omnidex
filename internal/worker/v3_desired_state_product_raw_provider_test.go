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

	contextPrompt, err := assemblyline.BuildApplicationContextQuestionInventoryPrompt(
		assemblyline.ApplicationContextQuestionInventoryInput{
			UserRequest: request, Context: context,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	authority := assemblyline.RepositoryRequirementInterpretationInput{
		UserRequest: request, Context: context,
	}
	inventoryPrompt, err := assemblyline.BuildRepositoryRequirementInventoryPrompt(authority)
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := assemblyline.DecodeRepositoryRequirementInventory(authority, request)
	if err != nil {
		t.Fatal(err)
	}
	relationPrompt, err := assemblyline.BuildRepositoryRequirementCandidateAuthorizationPrompt(
		assemblyline.RepositoryRequirementCandidateAuthorizationInput{
			Authority: authority, Inventory: inventory, CandidateIndex: 0,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, prompt, response string
		kind                   assemblyline.WorkKind
	}{
		{
			name: "context question inventory", prompt: contextPrompt,
			response: assemblyline.ApplicationNoRepositoryFactQuestionCandidates,
			kind:     assemblyline.WorkApplicationContextQuestionInventory,
		},
		{
			name: "requirement inventory", prompt: inventoryPrompt,
			response: request, kind: assemblyline.WorkRepositoryRequirementInventory,
		},
		{
			name: "requirement candidate relation", prompt: relationPrompt,
			response: assemblyline.RepositoryRequirementCandidateRequiresChange,
			kind:     assemblyline.WorkRepositoryRequirementCandidateAuthorization,
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
