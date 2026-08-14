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
	retirementCount := 0
	conversationCutoverCount := 0
	semanticGapCount := 0
	stationCallCount := 0
	channelAuthorityCount := 0
	webReviewStationCount := 0
	channelWorkspaceBindingCount := 0
	channelMessageRoleBoundsCount := 0
	conversationContextSelectionCount := 0
	repositoryGroundingStationCount := 0
	stationTerminalAuthorityCount := 0
	scrumTypedAuthorityCount := 0
	workerSkillPromotionCount := 0
	stationJSONAuthorityCount := 0
	workerSkillRetrievalOnlyCount := 0
	objectiveCompletionEvidenceCount := 0
	memoryObjectiveContextCount := 0
	repositoryFileStateCount := 0
	declarationArtifactBoundaryStationCount := 0
	artifactCandidateSelectionStationCount := 0
	knownArtifactTruthStationCount := 0
	projectPlanningRetirementCount := 0
	executablePipelineAuthorityCount := 0
	exactLifecycleFeedbackAuthorityCount := 0
	scrumChannelMessageRelationCount := 0
	scrumChannelOperationReceiptsCount := 0
	retiredExecutionAuthorityCount := 0
	portableRendererV2Count := 0
	portableRendererV3Count := 0
	for _, entry := range bundle.entries {
		if strings.HasPrefix(entry.name, "060_") {
			got[entry.name]++
		}
		if entry.name == "065_legacy_cognition_runtime_retirement.sql" {
			retirementCount++
		}
		if entry.name == "066_conversation_objective_cutover.sql" {
			conversationCutoverCount++
		}
		if entry.name == "067_semantic_gap_authority.sql" {
			semanticGapCount++
		}
		if entry.name == "068_station_call_authority.sql" {
			stationCallCount++
		}
		if entry.name == "069_channel_authority.sql" {
			channelAuthorityCount++
		}
		if entry.name == "070_web_claim_evidence_review_station.sql" {
			webReviewStationCount++
		}
		if entry.name == "071_channel_workspace_binding.sql" {
			channelWorkspaceBindingCount++
		}
		if entry.name == "072_channel_message_role_bounds.sql" {
			channelMessageRoleBoundsCount++
		}
		if entry.name == "073_conversation_context_selection_station.sql" {
			conversationContextSelectionCount++
		}
		if entry.name == "074_repository_grounding_stations.sql" {
			repositoryGroundingStationCount++
		}
		if entry.name == "075_station_terminal_receipt_authority.sql" {
			stationTerminalAuthorityCount++
		}
		if entry.name == "076_scrum_typed_authority.sql" {
			scrumTypedAuthorityCount++
		}
		if entry.name == "077_worker_skill_promotion_authority.sql" {
			workerSkillPromotionCount++
		}
		if entry.name == "078_station_json_authority.sql" {
			stationJSONAuthorityCount++
		}
		if entry.name == "079_worker_skill_retrieval_only.sql" {
			workerSkillRetrievalOnlyCount++
		}
		if entry.name == "080_objective_completion_evidence_authority.sql" {
			objectiveCompletionEvidenceCount++
		}
		if entry.name == "081_memory_objective_context_authority.sql" {
			memoryObjectiveContextCount++
		}
		if entry.name == "082_repository_mutation_file_state_transitions.sql" {
			repositoryFileStateCount++
		}
		if entry.name == "083_declaration_artifact_boundary_station.sql" {
			declarationArtifactBoundaryStationCount++
		}
		if entry.name == "084_artifact_candidate_selection_station.sql" {
			artifactCandidateSelectionStationCount++
		}
		if entry.name == "085_known_artifact_truth_station.sql" {
			knownArtifactTruthStationCount++
		}
		if entry.name == "086_project_planning_retirement.sql" {
			projectPlanningRetirementCount++
		}
		if entry.name == "087_executable_pipeline_authority.sql" {
			executablePipelineAuthorityCount++
		}
		if entry.name == "088_exact_lifecycle_feedback_authority.sql" {
			exactLifecycleFeedbackAuthorityCount++
		}
		if entry.name == "089_scrum_channel_message_relation.sql" {
			scrumChannelMessageRelationCount++
		}
		if entry.name == "090_scrum_channel_operation_receipts.sql" {
			scrumChannelOperationReceiptsCount++
		}
		if entry.name == "091_retired_execution_authority.sql" {
			retiredExecutionAuthorityCount++
		}
		if entry.name == "092_portable_renderer_v2.sql" {
			portableRendererV2Count++
		}
		if entry.name == "093_portable_renderer_v3.sql" {
			portableRendererV3Count++
		}
	}
	if len(bundle.entries) != 144 || len(got) != len(want) {
		t.Fatalf("checked migration counts total/provider=%d/%d want 144/%d",
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
	if retirementCount != 1 {
		t.Fatalf("checked retirement migration count=%d want 1", retirementCount)
	}
	if conversationCutoverCount != 1 {
		t.Fatalf("checked conversation cutover migration count=%d want 1", conversationCutoverCount)
	}
	if semanticGapCount != 1 || stationCallCount != 1 || channelAuthorityCount != 1 {
		t.Fatalf("checked horizontal authority migrations gap/call/channel=%d/%d/%d want 1/1/1",
			semanticGapCount, stationCallCount, channelAuthorityCount)
	}
	if webReviewStationCount != 1 {
		t.Fatalf("checked web review station migration count=%d want 1", webReviewStationCount)
	}
	if channelWorkspaceBindingCount != 1 {
		t.Fatalf("checked channel workspace binding migration count=%d want 1", channelWorkspaceBindingCount)
	}
	if channelMessageRoleBoundsCount != 1 {
		t.Fatalf("checked channel message role bounds migration count=%d want 1", channelMessageRoleBoundsCount)
	}
	if conversationContextSelectionCount != 1 {
		t.Fatalf("checked conversation context-selection migration count=%d want 1", conversationContextSelectionCount)
	}
	if repositoryGroundingStationCount != 1 {
		t.Fatalf("checked repository-grounding station migration count=%d want 1", repositoryGroundingStationCount)
	}
	if stationTerminalAuthorityCount != 1 || scrumTypedAuthorityCount != 1 ||
		workerSkillPromotionCount != 1 || stationJSONAuthorityCount != 1 ||
		workerSkillRetrievalOnlyCount != 1 || objectiveCompletionEvidenceCount != 1 ||
		memoryObjectiveContextCount != 1 || repositoryFileStateCount != 1 ||
		declarationArtifactBoundaryStationCount != 1 ||
		artifactCandidateSelectionStationCount != 1 ||
		knownArtifactTruthStationCount != 1 {
		t.Fatalf(
			"checked migrations 075..085 counts=%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d want all one",
			stationTerminalAuthorityCount, scrumTypedAuthorityCount,
			workerSkillPromotionCount, stationJSONAuthorityCount,
			workerSkillRetrievalOnlyCount, objectiveCompletionEvidenceCount,
			memoryObjectiveContextCount, repositoryFileStateCount,
			declarationArtifactBoundaryStationCount,
			artifactCandidateSelectionStationCount,
			knownArtifactTruthStationCount,
		)
	}
	if projectPlanningRetirementCount != 1 {
		t.Fatalf("checked project-planning retirement migration count=%d want 1", projectPlanningRetirementCount)
	}
	if executablePipelineAuthorityCount != 1 {
		t.Fatalf("checked executable-pipeline authority migration count=%d want 1", executablePipelineAuthorityCount)
	}
	if exactLifecycleFeedbackAuthorityCount != 1 {
		t.Fatalf("checked exact-lifecycle-feedback migration count=%d want 1", exactLifecycleFeedbackAuthorityCount)
	}
	if scrumChannelMessageRelationCount != 1 || scrumChannelOperationReceiptsCount != 1 ||
		retiredExecutionAuthorityCount != 1 || portableRendererV2Count != 1 ||
		portableRendererV3Count != 1 {
		t.Fatalf("checked migrations 089/090/091/092/093 counts=%d/%d/%d/%d/%d want all one",
			scrumChannelMessageRelationCount, scrumChannelOperationReceiptsCount,
			retiredExecutionAuthorityCount, portableRendererV2Count, portableRendererV3Count)
	}
}
