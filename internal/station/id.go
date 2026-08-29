package station

import "fmt"

// ID names one bounded semantic transformation. It is model routing data, not
// a persona, tool owner, workflow, or completion authority.
type ID string

const (
	ContextRelevance                     ID = "context_relevance"
	ContextMinification                  ID = "context_minification"
	ConversationObjectiveKind            ID = "conversation_objective_kind"
	ConversationResponse                 ID = "conversation_response"
	RoleplayCanonExtraction              ID = "roleplay_canon_extraction"
	RoleplayOngoingAction                ID = "roleplay_ongoing_action"
	GroundedAnswer                       ID = "grounded_answer"
	DatabaseSchemaSelection              ID = "database_schema_selection"
	DatabaseQueryIntent                  ID = "database_query_intent"
	DatabaseEvidenceGap                  ID = "database_evidence_gap"
	DatabaseJoinPathSelection            ID = "database_join_path_selection"
	RepositoryEvidenceRelevance          ID = "repository_evidence_relevance"
	WebRelevance                         ID = "web_relevance"
	WebGroundedSynthesis                 ID = "web_grounded_synthesis"
	CodingSurface                        ID = "coding_surface"
	CodingRequirements                   ID = "coding_requirements"
	CodingProjectStackConstraint         ID = "coding_project_stack_constraint"
	CodingServiceContinuedAvailability   ID = "coding_service_continued_availability"
	CodingServicePersistenceDestination  ID = "coding_service_persistence_destination"
	CodingServiceStateLifetime           ID = "coding_service_state_lifetime"
	CodingApplicationStateFieldCoverage  ID = "coding_application_state_field_coverage"
	CodingApplicationStateFieldPurpose   ID = "coding_application_state_field_purpose"
	CodingApplicationStateFieldKind      ID = "coding_application_state_field_kind"
	CodingApplicationRecordFieldCoverage ID = "coding_application_record_field_coverage"
	CodingApplicationRecordFieldPurpose  ID = "coding_application_record_field_purpose"
	CodingApplicationRecordFieldKind     ID = "coding_application_record_field_kind"
	CodingServiceEndpointRequirement     ID = "coding_service_endpoint_requirement"
	CodingServiceEndpointExposure        ID = "coding_service_endpoint_exposure"
	CodingServiceEndpointMethod          ID = "coding_service_endpoint_method"
	CodingServiceEndpointRouteTemplate   ID = "coding_service_endpoint_route_template"
	CodingServiceEndpointRequestMedia    ID = "coding_service_endpoint_request_media"
	CodingServiceEndpointResponseMedia   ID = "coding_service_endpoint_response_media"
	CodingServiceEndpointSuccessStatus   ID = "coding_service_endpoint_success_status"
	CodingTargetTree                     ID = "coding_target_tree"
	CodingArtifactHandling               ID = "coding_artifact_handling"
	CodingRepositoryArtifactAbsence      ID = "coding_repository_artifact_absence"
	CodingPlainTextArtifactCreation      ID = "coding_plain_text_artifact_creation"
	CodingDeclarationArtifactBoundary    ID = "coding_declaration_artifact_boundary"
	CodingArtifactCandidateSelection     ID = "coding_artifact_candidate_selection"
	CodingCapabilityRelation             ID = "coding_capability_relation"
	CodingSkillSelection                 ID = "coding_skill_selection"
	CodingRuntimeCapabilitySelection     ID = "coding_runtime_capability_selection"
	CodingFragment                       ID = "coding_fragment"
	CodingFragmentRepairGuidance         ID = "coding_fragment_repair_guidance"
	CodingFragmentCorrection             ID = "coding_fragment_correction"
	CodingRepositoryChange               ID = "coding_repository_change_surface"
)

var registered = [...]ID{
	ContextRelevance,
	ContextMinification,
	ConversationObjectiveKind,
	ConversationResponse,
	RoleplayCanonExtraction,
	RoleplayOngoingAction,
	GroundedAnswer,
	DatabaseSchemaSelection,
	DatabaseQueryIntent,
	DatabaseEvidenceGap,
	DatabaseJoinPathSelection,
	RepositoryEvidenceRelevance,
	WebRelevance,
	WebGroundedSynthesis,
	CodingSurface,
	CodingRequirements,
	CodingProjectStackConstraint,
	CodingServiceContinuedAvailability,
	CodingServicePersistenceDestination,
	CodingServiceStateLifetime,
	CodingApplicationStateFieldCoverage,
	CodingApplicationStateFieldPurpose,
	CodingApplicationStateFieldKind,
	CodingApplicationRecordFieldCoverage,
	CodingApplicationRecordFieldPurpose,
	CodingApplicationRecordFieldKind,
	CodingServiceEndpointRequirement,
	CodingServiceEndpointExposure,
	CodingServiceEndpointMethod,
	CodingServiceEndpointRouteTemplate,
	CodingServiceEndpointRequestMedia,
	CodingServiceEndpointResponseMedia,
	CodingServiceEndpointSuccessStatus,
	CodingTargetTree,
	CodingArtifactHandling,
	CodingRepositoryArtifactAbsence,
	CodingPlainTextArtifactCreation,
	CodingDeclarationArtifactBoundary,
	CodingArtifactCandidateSelection,
	CodingCapabilityRelation,
	CodingSkillSelection,
	CodingRuntimeCapabilitySelection,
	CodingFragment,
	CodingFragmentRepairGuidance,
	CodingFragmentCorrection,
	CodingRepositoryChange,
}

var registeredSet = func() map[ID]struct{} {
	set := make(map[ID]struct{}, len(registered))
	for _, id := range registered {
		set[id] = struct{}{}
	}
	return set
}()

func All() []ID {
	ids := make([]ID, len(registered))
	copy(ids, registered[:])
	return ids
}

func (id ID) String() string {
	return string(id)
}

func (id ID) Validate() error {
	if _, ok := registeredSet[id]; ok {
		return nil
	}
	return fmt.Errorf("unregistered semantic station %q", id)
}
