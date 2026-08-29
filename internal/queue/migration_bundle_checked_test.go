package queue

import (
	"strings"
	"testing"
)

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
	tokenizerProfileAuthorityCount := 0
	stationOutputProjectionCount := 0
	stationOutputLimitModeCount := 0
	stationPromptTransportCount := 0
	acceptanceGroundingReviewStationCount := 0
	stationResponseSchemaResourceCount := 0
	codingWorkloadReviewStationCount := 0
	qwen25CoderTokenizerProfileCount := 0
	qwen3NativeTokenizerProfileCount := 0
	legacyCoderTokenizerProfileCount := 0
	codingFragmentRepairGuidanceCount := 0
	applicationFrontDoorStationCount := 0
	dataSourceRelationalAuthorityCount := 0
	databaseCognitionAuthorityCount := 0
	roleplayCanonAuthorityCount := 0
	roleplaySimulationAuthorityCount := 0
	roleplayResearchAuthorityCount := 0
	roleplayTerminalSimulationPublicationCount := 0
	delegatedDataSourceAuthorityCount := 0
	roleplayCharacterLibraryCount := 0
	ollamaModelDownloadAuthorityCount := 0
	roleplayCharacterGenerationAuthorityCount := 0
	roleplayVoicePreservationStationCount := 0
	roleplayNarrativeContinuityStationCount := 0
	contextSieveCutoverCount := 0
	roleplayUserTurnAuthorityCount := 0
	roleplayVoiceRetirementCount := 0
	projectStackConstraintStationCount := 0
	serviceEndpointContractStationCount := 0
	serviceEndpointRequirementStationCount := 0
	acceptanceGroundingRetirementCount := 0
	serviceEndpointLeafStationCount := 0
	serviceStateLifetimeStationCount := 0
	serviceDeploymentIntentStationCount := 0
	serviceDeploymentSemanticSplitCount := 0
	generatedWorkloadDeploymentJournalCount := 0
	generatedWorkloadDeploymentEvidenceCount := 0
	generatedWorkloadProjectDeploymentHeadCount := 0
	generatedWorkloadDeploymentRecoveryCount := 0
	generatedWorkloadDeploymentNamespacePreflightCount := 0
	serviceStateInterfaceStationCount := 0
	roleplayPortableResultReuseV2Count := 0
	stationGapOpeningPortableEnvelopeV2Count := 0
	workspaceMutationPipelineActionAuthorityCount := 0
	jobExecutionIdentityImmutabilityCount := 0
	artifactSemanticRelationSplitCount := 0
	responseSchemaAuthorityRetirementCount := 0
	semanticUncertaintyContractAuthorityCount := 0
	llmEvidenceTransportIdentityCutoverCount := 0
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
		if entry.name == tokenizerProfileAuthorityMigration {
			tokenizerProfileAuthorityCount++
		}
		if entry.name == "095_station_output_artifact_projection.sql" {
			stationOutputProjectionCount++
		}
		if entry.name == stationOutputLimitModeMigration {
			stationOutputLimitModeCount++
		}
		if entry.name == stationPromptTransportMigration {
			stationPromptTransportCount++
		}
		if entry.name == "098_application_acceptance_grounding_review_station.sql" {
			acceptanceGroundingReviewStationCount++
		}
		if entry.name == stationResponseSchemaResourceMigration {
			stationResponseSchemaResourceCount++
		}
		if entry.name == "101_coding_workload_review_station.sql" {
			codingWorkloadReviewStationCount++
		}
		if entry.name == qwen25CoderTokenizerProfileMigration {
			qwen25CoderTokenizerProfileCount++
		}
		if entry.name == qwen3NativeTokenizerProfileMigration {
			qwen3NativeTokenizerProfileCount++
		}
		if entry.name == legacyCoderTokenizerProfileMigration {
			legacyCoderTokenizerProfileCount++
		}
		if entry.name == codingFragmentRepairGuidanceMigration {
			codingFragmentRepairGuidanceCount++
		}
		if entry.name == applicationFrontDoorStationMigration {
			applicationFrontDoorStationCount++
		}
		if entry.name == "115_data_source_relational_authority.sql" {
			dataSourceRelationalAuthorityCount++
		}
		if entry.name == "116_database_cognition_authority.sql" {
			databaseCognitionAuthorityCount++
		}
		if entry.name == "117_roleplay_canon_authority.sql" {
			roleplayCanonAuthorityCount++
		}
		if entry.name == "118_roleplay_simulation_authority.sql" {
			roleplaySimulationAuthorityCount++
		}
		if entry.name == "119_roleplay_research_authority.sql" {
			roleplayResearchAuthorityCount++
		}
		if entry.name == "120_roleplay_terminal_simulation_publication.sql" {
			roleplayTerminalSimulationPublicationCount++
		}
		if entry.name == "121_delegated_data_source_authority.sql" {
			delegatedDataSourceAuthorityCount++
		}
		if entry.name == "122_roleplay_character_library.sql" {
			roleplayCharacterLibraryCount++
		}
		if entry.name == "123_ollama_model_download_authority.sql" {
			ollamaModelDownloadAuthorityCount++
		}
		if entry.name == "124_roleplay_character_generation_authority.sql" {
			roleplayCharacterGenerationAuthorityCount++
		}
		if entry.name == "125_roleplay_voice_preservation_station.sql" {
			roleplayVoicePreservationStationCount++
		}
		if entry.name == "126_roleplay_narrative_continuity_station.sql" {
			roleplayNarrativeContinuityStationCount++
		}
		if entry.name == contextSieveCutoverMigration {
			contextSieveCutoverCount++
		}
		if entry.name == roleplayUserTurnAuthorityMigration {
			roleplayUserTurnAuthorityCount++
		}
		if entry.name == "129_retire_roleplay_voice_rewrite.sql" {
			roleplayVoiceRetirementCount++
		}
		if entry.name == "133_application_project_stack_constraint_station.sql" {
			projectStackConstraintStationCount++
		}
		if entry.name == applicationServiceEndpointContractMigration {
			serviceEndpointContractStationCount++
		}
		if entry.name == applicationServiceEndpointRequirementMigration {
			serviceEndpointRequirementStationCount++
		}
		if entry.name == applicationAcceptanceGroundingRetirementMigration {
			acceptanceGroundingRetirementCount++
		}
		if entry.name == applicationServiceEndpointLeafMigration {
			serviceEndpointLeafStationCount++
		}
		if entry.name == applicationServiceStateLifetimeMigration {
			serviceStateLifetimeStationCount++
		}
		if entry.name == applicationServiceDeploymentIntentMigration {
			serviceDeploymentIntentStationCount++
		}
		if entry.name == applicationServiceDeploymentSemanticSplitMigration {
			serviceDeploymentSemanticSplitCount++
		}
		if entry.name == "140_generated_workload_deployment_journal.sql" {
			generatedWorkloadDeploymentJournalCount++
		}
		if entry.name == "141_generated_workload_deployment_evidence_rail.sql" {
			generatedWorkloadDeploymentEvidenceCount++
		}
		if entry.name == "142_generated_workload_project_deployment_head.sql" {
			generatedWorkloadProjectDeploymentHeadCount++
		}
		if entry.name == applicationServiceStateInterfaceMigration {
			serviceStateInterfaceStationCount++
		}
		if entry.name == "144_generated_workload_deployment_recovery.sql" {
			generatedWorkloadDeploymentRecoveryCount++
		}
		if entry.name == "145_generated_workload_deployment_namespace_preflight.sql" {
			generatedWorkloadDeploymentNamespacePreflightCount++
		}
		if entry.name == roleplayPortableResultReuseV2Migration {
			roleplayPortableResultReuseV2Count++
		}
		if entry.name == stationGapOpeningPortableEnvelopeV2Migration {
			stationGapOpeningPortableEnvelopeV2Count++
		}
		if entry.name == workspaceMutationPipelineActionAuthorityMigration {
			workspaceMutationPipelineActionAuthorityCount++
		}
		if entry.name == jobExecutionIdentityImmutabilityMigration {
			jobExecutionIdentityImmutabilityCount++
		}
		if entry.name == artifactSemanticRelationSplitMigration {
			artifactSemanticRelationSplitCount++
		}
		if entry.name == responseSchemaAuthorityRetirementMigration {
			responseSchemaAuthorityRetirementCount++
		}
		if entry.name == "182_semantic_uncertainty_contract_authority.sql" {
			semanticUncertaintyContractAuthorityCount++
		}
		if entry.name == "183_llm_evidence_transport_identity_cutover.sql" {
			llmEvidenceTransportIdentityCutoverCount++
		}
	}
	if len(bundle.entries) != 234 || len(got) != len(want) {
		t.Fatalf("checked migration counts total/provider=%d/%d want 234/%d",
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
	if dataSourceRelationalAuthorityCount != 1 {
		t.Fatalf("checked relational data-source authority migration count=%d want 1", dataSourceRelationalAuthorityCount)
	}
	if roleplayPortableResultReuseV2Count != 1 {
		t.Fatalf(
			"checked roleplay portable-result-reuse V2 migration count=%d want 1",
			roleplayPortableResultReuseV2Count,
		)
	}
	if stationGapOpeningPortableEnvelopeV2Count != 1 {
		t.Fatalf(
			"checked station-gap portable-envelope V2 migration count=%d want 1",
			stationGapOpeningPortableEnvelopeV2Count,
		)
	}
	if workspaceMutationPipelineActionAuthorityCount != 1 {
		t.Fatalf(
			"checked workspace-mutation pipeline/action authority migration count=%d want 1",
			workspaceMutationPipelineActionAuthorityCount,
		)
	}
	if jobExecutionIdentityImmutabilityCount != 1 {
		t.Fatalf(
			"checked job execution identity immutability migration count=%d want 1",
			jobExecutionIdentityImmutabilityCount,
		)
	}
	if artifactSemanticRelationSplitCount != 1 {
		t.Fatalf(
			"checked artifact semantic relation split migration count=%d want 1",
			artifactSemanticRelationSplitCount,
		)
	}
	if responseSchemaAuthorityRetirementCount != 1 {
		t.Fatalf(
			"checked response-schema authority retirement migration count=%d want 1",
			responseSchemaAuthorityRetirementCount,
		)
	}
	if semanticUncertaintyContractAuthorityCount != 1 {
		t.Fatalf(
			"checked semantic uncertainty contract authority migration count=%d want 1",
			semanticUncertaintyContractAuthorityCount,
		)
	}
	if llmEvidenceTransportIdentityCutoverCount != 1 {
		t.Fatalf(
			"checked LLM evidence transport identity cutover migration count=%d want 1",
			llmEvidenceTransportIdentityCutoverCount,
		)
	}
	if databaseCognitionAuthorityCount != 1 || roleplayCanonAuthorityCount != 1 ||
		roleplaySimulationAuthorityCount != 1 || roleplayResearchAuthorityCount != 1 ||
		roleplayTerminalSimulationPublicationCount != 1 || delegatedDataSourceAuthorityCount != 1 ||
		roleplayCharacterLibraryCount != 1 || ollamaModelDownloadAuthorityCount != 1 ||
		roleplayCharacterGenerationAuthorityCount != 1 || roleplayVoicePreservationStationCount != 1 ||
		roleplayNarrativeContinuityStationCount != 1 || contextSieveCutoverCount != 1 ||
		roleplayUserTurnAuthorityCount != 1 || roleplayVoiceRetirementCount != 1 ||
		projectStackConstraintStationCount != 1 {
		t.Fatalf(
			"checked database/roleplay/delegated/context authority migration counts=%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d want all one",
			databaseCognitionAuthorityCount, roleplayCanonAuthorityCount,
			roleplaySimulationAuthorityCount, roleplayResearchAuthorityCount,
			roleplayTerminalSimulationPublicationCount, delegatedDataSourceAuthorityCount,
			roleplayCharacterLibraryCount, ollamaModelDownloadAuthorityCount,
			roleplayCharacterGenerationAuthorityCount, roleplayVoicePreservationStationCount,
			roleplayNarrativeContinuityStationCount, contextSieveCutoverCount,
			roleplayUserTurnAuthorityCount, roleplayVoiceRetirementCount,
			projectStackConstraintStationCount,
		)
	}
	if conversationCutoverCount != 1 {
		t.Fatalf("checked conversation cutover migration count=%d want 1", conversationCutoverCount)
	}
	if serviceEndpointContractStationCount != 1 {
		t.Fatalf(
			"checked service endpoint contract migration count=%d want 1",
			serviceEndpointContractStationCount,
		)
	}
	if serviceEndpointRequirementStationCount != 1 {
		t.Fatalf(
			"checked service endpoint requirement migration count=%d want 1",
			serviceEndpointRequirementStationCount,
		)
	}
	if acceptanceGroundingRetirementCount != 1 {
		t.Fatalf(
			"checked acceptance-grounding retirement migration count=%d want 1",
			acceptanceGroundingRetirementCount,
		)
	}
	if serviceEndpointLeafStationCount != 1 {
		t.Fatalf(
			"checked service endpoint leaf migration count=%d want 1",
			serviceEndpointLeafStationCount,
		)
	}
	if serviceStateLifetimeStationCount != 1 {
		t.Fatalf(
			"checked service state lifetime migration count=%d want 1",
			serviceStateLifetimeStationCount,
		)
	}
	if serviceDeploymentIntentStationCount != 1 {
		t.Fatalf(
			"checked service deployment intent migration count=%d want 1",
			serviceDeploymentIntentStationCount,
		)
	}
	if serviceDeploymentSemanticSplitCount != 1 {
		t.Fatalf(
			"checked service deployment semantic split migration count=%d want 1",
			serviceDeploymentSemanticSplitCount,
		)
	}
	if generatedWorkloadDeploymentJournalCount != 1 {
		t.Fatalf(
			"checked generated workload deployment journal migration count=%d want 1",
			generatedWorkloadDeploymentJournalCount,
		)
	}
	if generatedWorkloadDeploymentEvidenceCount != 1 ||
		generatedWorkloadProjectDeploymentHeadCount != 1 ||
		serviceStateInterfaceStationCount != 1 ||
		generatedWorkloadDeploymentRecoveryCount != 1 ||
		generatedWorkloadDeploymentNamespacePreflightCount != 1 {
		t.Fatalf(
			"checked deployment evidence/head/state-interface/recovery/namespace-preflight migration counts=%d/%d/%d/%d/%d want all one",
			generatedWorkloadDeploymentEvidenceCount,
			generatedWorkloadProjectDeploymentHeadCount,
			serviceStateInterfaceStationCount,
			generatedWorkloadDeploymentRecoveryCount,
			generatedWorkloadDeploymentNamespacePreflightCount,
		)
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
		portableRendererV3Count != 1 || tokenizerProfileAuthorityCount != 1 ||
		stationOutputProjectionCount != 1 || stationOutputLimitModeCount != 1 ||
		stationPromptTransportCount != 1 || acceptanceGroundingReviewStationCount != 1 ||
		stationResponseSchemaResourceCount != 1 || codingWorkloadReviewStationCount != 1 ||
		qwen25CoderTokenizerProfileCount != 1 || qwen3NativeTokenizerProfileCount != 1 ||
		legacyCoderTokenizerProfileCount != 1 || codingFragmentRepairGuidanceCount != 1 ||
		applicationFrontDoorStationCount != 1 {
		t.Fatalf("checked migrations 089..106 counts=%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d/%d want all one",
			scrumChannelMessageRelationCount, scrumChannelOperationReceiptsCount,
			retiredExecutionAuthorityCount, portableRendererV2Count, portableRendererV3Count,
			tokenizerProfileAuthorityCount, stationOutputProjectionCount,
			stationOutputLimitModeCount, stationPromptTransportCount,
			acceptanceGroundingReviewStationCount, stationResponseSchemaResourceCount,
			codingWorkloadReviewStationCount, qwen25CoderTokenizerProfileCount,
			qwen3NativeTokenizerProfileCount, legacyCoderTokenizerProfileCount,
			codingFragmentRepairGuidanceCount, applicationFrontDoorStationCount)
	}
}
