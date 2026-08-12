package queue

import (
	"path/filepath"
	"strings"
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

func TestCheckedMigrationBundleFreezesExactProviderSplitSet(t *testing.T) {
	bundle := loadCheckedMigrationBundle(t)
	want := []string{
		"060_cognition_exact_policy_input_authority.sql",
		"060_cognition_exact_provider_content_encoding.sql",
		"060_cognition_exact_provider_usage.sql",
		"060_cognition_exact_provider_usage_a_response_evidence.sql",
		"060_cognition_exact_provider_usage_aa_generation_wire_guards.sql",
		"060_cognition_exact_provider_usage_ab_generation_wire_semantics.sql",
		"060_cognition_exact_provider_usage_b_generation_evidence.sql",
		"060_cognition_exact_provider_usage_c_response_capture.sql",
		"060_cognition_exact_provider_usage_d_response_projection.sql",
		"060_cognition_exact_provider_usage_e_projection.sql",
		"060_cognition_exact_provider_usage_f_result_shape.sql",
		"060_cognition_exact_provider_usage_g_budget_authority.sql",
		"060_cognition_exact_provider_usage_ga_call_attempt_types.sql",
		"060_cognition_exact_provider_usage_gb_call_result_types.sql",
		"060_cognition_exact_provider_usage_gc_provider_receipt.sql",
		"060_cognition_exact_provider_usage_gd_result_semantics.sql",
		"060_cognition_exact_provider_usage_guards.sql",
		"060_cognition_provider_identity_evidence.sql",
		"060_cognition_provider_identity_evidence_c_json_types.sql",
		"060_cognition_provider_identity_evidence_derivation.sql",
		"060_cognition_provider_identity_evidence_guards.sql",
		"060_cognition_provider_identity_evidence_guards_b_associations.sql",
		"060_cognition_provider_identity_evidence_guards_c_brain.sql",
		"060_cognition_provider_identity_evidence_guards_d_attestations.sql",
		"060_cognition_provider_identity_evidence_guards_e_observation.sql",
		"060_cognition_provider_identity_evidence_guards_f_failure_proof.sql",
		"060_cognition_provider_identity_evidence_guards_g_associations.sql",
		"060_cognition_provider_process_failure_receipts.sql",
		"060_cognition_provider_process_failure_receipts_a_exact.sql",
		"060_cognition_provider_process_observation.sql",
		"060_cognition_provider_process_observation_a_receipt.sql",
		"060_cognition_provider_process_observation_guards.sql",
		"060_cognition_provider_process_observation_z_failure_outcomes.sql",
		"060_cognition_provider_process_observation_zz_failure_terminal.sql",
		"060_cognition_provider_process_observation_zzz_episode_start_totality.sql",
		"060_cognition_provider_process_observation_zzzz_bootstrap_trace_totality.sql",
		"060_cognition_provider_process_observation_zzzzz_postseal_replay_audit.sql",
		"060_cognition_provider_process_observation_zzzzz_postseal_replay_audit_a_associations.sql",
		"060_cognition_provider_process_observation_zzzzz_postseal_replay_audit_b_outcomes.sql",
	}
	got := make(map[string]int)
	for _, entry := range bundle.entries {
		if strings.HasPrefix(entry.name, "060_") {
			got[entry.name]++
		}
	}
	if len(bundle.entries) != 115 || len(got) != len(want) {
		t.Fatalf("checked migration counts total/provider=%d/%d want 115/%d",
			len(bundle.entries), len(got), len(want))
	}
	for _, name := range want {
		if got[name] != 1 {
			t.Fatalf("checked provider migration %q count=%d want 1", name, got[name])
		}
		delete(got, name)
	}
	if len(got) != 0 {
		t.Fatalf("checked bundle contains unexpected provider migrations: %v", got)
	}
}
