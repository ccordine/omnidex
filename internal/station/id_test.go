package station

import "testing"

func TestIDRejectsUnregisteredSemanticAuthority(t *testing.T) {
	t.Parallel()

	if err := ID("planner_specialist").Validate(); err == nil {
		t.Fatal("persona-shaped station ID was accepted")
	}
	if err := CodingFragment.Validate(); err != nil {
		t.Fatalf("registered leaf station rejected: %v", err)
	}
	if err := ID("objective_advisory").Validate(); err == nil {
		t.Fatal("retired objective advisory station was accepted")
	}
}

func TestAllContainsOnlyUniqueRegisteredStations(t *testing.T) {
	t.Parallel()

	seen := map[ID]struct{}{}
	for _, id := range All() {
		if err := id.Validate(); err != nil {
			t.Fatal(err)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate station %q", id)
		}
		seen[id] = struct{}{}
	}
	if len(seen) != 45 {
		t.Fatalf("registered stations=%d want 45", len(seen))
	}
}

func TestRetiredContextStationsAreUnregistered(t *testing.T) {
	t.Parallel()

	for _, retired := range []ID{
		"conversation_context_selection",
		"memory_context_selection",
		"roleplay_narrative_continuity",
		"roleplay_voice_rewrite",
		"roleplay_voice_preservation",
		"coding_workload_review",
		"coding_service_endpoint_contract",
		"coding_service_state_interface",
		"repository_grounded_review",
		"repository_grounded_correction",
		"context_search_terms",
		"coding_repository_search_term",
		"web_grounded_synthesis_correction",
		"web_claim_evidence_review",
		"coding_known_artifact_truth",
		"coding_runtime_capability_selection",
		"database_evidence_refinement",
	} {
		if err := retired.Validate(); err == nil {
			t.Fatalf("retired context station %q remains registered", retired)
		}
	}
}
