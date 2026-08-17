package assemblyline

import (
	"strings"
	"testing"
)

func TestTargetTreeDeclarationDiffsCreateMoveModifyRetainAndDelete(t *testing.T) {
	input := targetTreeFixtureInput()
	target, err := DecodeTargetTreeCandidate(input, `{
        "schema":"omnidex.target-tree.v1",
        "artifacts":[
          {"path":"src/application.ts","kind":"implementation","purpose":"compose the application","requirement_ids":["requirement_002"],"existing_artifact_id":"artifact_app","new_key":""},
          {"path":"src/counter.ts","kind":"implementation","purpose":"provide the requested counter behavior","requirement_ids":["requirement_001"],"existing_artifact_id":"artifact_counter","new_key":""},
          {"path":"tests/counter.test.ts","kind":"verification","purpose":"verify the requested counter behavior","requirement_ids":["requirement_001"],"existing_artifact_id":"","new_key":"new_test"},
          {"path":"tests/application.test.ts","kind":"verification","purpose":"verify application composition","requirement_ids":["requirement_002"],"existing_artifact_id":"","new_key":"new_app_test"}
        ]
    }`)
	if err != nil {
		t.Fatalf("decode target tree: %v", err)
	}
	transitions, err := DiffTargetTree(input, target)
	if err != nil {
		t.Fatalf("diff target tree: %v", err)
	}
	got := make(map[string]TargetTreeTransitionKind, len(transitions))
	for _, transition := range transitions {
		got[transition.ArtifactID] = transition.Kind
	}
	for artifactID, want := range map[string]TargetTreeTransitionKind{
		"artifact_app":     TargetTreeRetain,
		"artifact_counter": TargetTreeMove,
		"new:new_test":     TargetTreeCreate,
		"artifact_stale":   TargetTreeDelete,
	} {
		if got[artifactID] != want {
			t.Fatalf("transition for %s = %q, want %q", artifactID, got[artifactID], want)
		}
	}
}

func TestTargetTreeDeclarationMakesRequirementBindingChangeModify(t *testing.T) {
	input := targetTreeFixtureInput()
	input.Current[0].RequirementIDs = []string{"requirement_002"}
	target, err := DecodeTargetTreeCandidate(input, `{
        "schema":"omnidex.target-tree.v1",
        "artifacts":[
          {"path":"src/application.ts","kind":"implementation","purpose":"compose the application shell","requirement_ids":["requirement_002"],"existing_artifact_id":"artifact_app","new_key":""},
          {"path":"src/Counter.ts","kind":"implementation","purpose":"provide the requested counter behavior","requirement_ids":["requirement_001"],"existing_artifact_id":"artifact_counter","new_key":""},
          {"path":"src/stale.ts","kind":"verification","purpose":"verify the counter behavior","requirement_ids":["requirement_001"],"existing_artifact_id":"artifact_stale","new_key":""},
          {"path":"tests/application.test.ts","kind":"verification","purpose":"verify application composition","requirement_ids":["requirement_002"],"existing_artifact_id":"","new_key":"new_application_test"}
        ]
    }`)
	if err != nil {
		t.Fatalf("decode target tree: %v", err)
	}
	transitions, err := DiffTargetTree(input, target)
	if err != nil {
		t.Fatalf("diff target tree: %v", err)
	}
	for _, transition := range transitions {
		if transition.ArtifactID == "artifact_app" && transition.Kind != TargetTreeModify {
			t.Fatalf("application transition = %q, want modify", transition.Kind)
		}
	}
}

func TestTargetTreeRejectsUnknownIdentityAndTraversal(t *testing.T) {
	input := targetTreeFixtureInput()
	_, err := DecodeTargetTreeCandidate(input, `{
        "schema":"omnidex.target-tree.v1",
        "artifacts":[
          {"path":"../escape.ts","kind":"implementation","purpose":"escape","requirement_ids":["requirement_001"],"existing_artifact_id":"unknown","new_key":""}
        ]
    }`)
	if err == nil || !strings.Contains(err.Error(), "normalized relative") {
		t.Fatalf("error = %v, want normalized path rejection", err)
	}
}

func TestTargetTreePortableEnvelopeContainsOnlyDeclarationContext(t *testing.T) {
	job, err := NewTargetTreeJob(targetTreeFixtureInput())
	if err != nil {
		t.Fatalf("new job: %v", err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatalf("render job: %v", err)
	}
	for _, forbidden := range []string{"mkdir", "write_file", "task ledger", "completion"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("prompt leaks forbidden operation term %q: %s", forbidden, prompt)
		}
	}
	if !strings.Contains(prompt, "CURRENT_ARTIFACT_INVENTORY_JSON") || schema == nil {
		t.Fatalf("target tree prompt/schema missing declaration boundary")
	}
}

func targetTreeFixtureInput() TargetTreeInput {
	return TargetTreeInput{
		Objective: "Build the requested counter application.",
		Requirements: []TargetTreeRequirement{
			{ID: "requirement_001", Statement: "Provide a counter."},
			{ID: "requirement_002", Statement: "Compose the application."},
		},
		Current: []CurrentTargetArtifact{
			{ID: "artifact_app", Path: "src/application.ts", Kind: TargetArtifactImplementation, Purpose: "compose the application", RequirementIDs: []string{"requirement_002"}},
			{ID: "artifact_counter", Path: "src/Counter.ts", Kind: TargetArtifactImplementation, Purpose: "provide the requested counter behavior", RequirementIDs: []string{"requirement_001"}},
			{ID: "artifact_stale", Path: "src/stale.ts", Kind: TargetArtifactVerification, Purpose: "legacy helper", RequirementIDs: []string{"requirement_001"}},
		},
	}
}
