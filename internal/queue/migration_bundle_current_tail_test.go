package queue

import (
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/version"
)

func loadCheckedMigrationBundle(t testing.TB) MigrationBundle {
	t.Helper()
	directory, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := LoadMigrationBundle(directory, version.MigrationsSHA256)
	if err != nil {
		t.Fatalf("load checked migration bundle: %v", err)
	}
	return bundle
}

func TestCheckedMigrationBundleFreezesCurrentTail(t *testing.T) {
	bundle := loadCheckedMigrationBundle(t)
	const currentTail = "188_portable_renderer_v8_requirement_coverage_authority.sql"
	if tail := bundle.entries[len(bundle.entries)-1].name; tail != currentTail {
		t.Fatalf("checked migration tail=%q want %q", tail, currentTail)
	}
	want := map[string]int{
		"146_generated_workload_deployment_namespace_requalification.sql": 0,
		"147_generated_workload_deployment_authority_hardening.sql":       0,
		"148_roleplay_initiative_time_authority.sql":                      0,
		"149_roleplay_ongoing_action_authority.sql":                       0,
		"150_roleplay_user_persona_scene_authority.sql":                   0,
		"151_roleplay_transition_observer_authority.sql":                  0,
		"152_roleplay_user_canon_provenance.sql":                          0,
		"153_roleplay_user_turn_contribution_kind_authority.sql":          0,
		"154_roleplay_semantic_tokenizer_profile_authority.sql":           0,
		"155_roleplay_portable_result_reuse.sql":                          0,
		"156_roleplay_semantic_model_route.sql":                           0,
		"157_roleplay_user_canon_modality_authority.sql":                  0,
		"158_scrum_message_tail_bounded_index.sql":                        0,
		"159_workspace_mutation_journal_cutover.sql":                      0,
		"160_application_service_deployment_semantic_split.sql":           0,
		"161_source_declaration_projection_authority.sql":                 0,
		"162_source_projection_identity.sql":                              0,
		"163_raw_portable_response_transport.sql":                         0,
		"164_qwen35_chatml_raw_transport_v2.sql":                          0,
		"165_portable_response_output_ceiling.sql":                        0,
		"166_roleplay_portable_result_reuse_v2.sql":                       0,
		"167_station_gap_opening_portable_envelope_v2.sql":                0,
		"168_workspace_mutation_pipeline_action_authority.sql":            0,
		"169_job_execution_identity_immutability.sql":                     0,
		"170_single_output_and_ledger_authority_retirement.sql":           0,
		"171_current_semantic_station_authority.sql":                      0,
		"172_generic_claim_persistence_retirement.sql":                    0,
		"173_dormant_telemetry_authority_retirement.sql":                  0,
		"174_model_call_repair_metric_retirement.sql":                     0,
		"175_objective_citation_requirement_authority_bindings.sql":       0,
		"176_scrum_auto_play_through_setting_retirement.sql":              0,
		"177_narrow_service_semantic_leaf_authority.sql":                  0,
		"178_exact_source_response_authority.sql":                         0,
		"179_workspace_mutation_project_location_authority.sql":           0,
		"180_artifact_semantic_relation_split.sql":                        0,
		"181_response_schema_authority_retirement.sql":                    0,
		"182_semantic_uncertainty_contract_authority.sql":                 0,
		"183_llm_evidence_transport_identity_cutover.sql":                 0,
		"184_fragment_generation_output_limit_replacement.sql":            0,
		"185_portable_renderer_v6.sql":                                    0,
		"186_runtime_capability_selection_station.sql":                    0,
		"187_portable_renderer_v7_application_intent_uncertainty_v2.sql":  0,
		currentTail: 0,
	}
	for _, entry := range bundle.entries {
		if _, tracked := want[entry.name]; tracked {
			want[entry.name]++
		}
	}
	for name, count := range want {
		if count != 1 {
			t.Fatalf("checked migration %q count=%d want 1", name, count)
		}
	}
}
