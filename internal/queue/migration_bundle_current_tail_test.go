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
