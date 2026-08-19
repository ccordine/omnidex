package station

import "fmt"

// ID names one bounded semantic transformation. It is model routing data, not
// a persona, tool owner, workflow, or completion authority.
type ID string

const (
	ConversationContextSelection      ID = "conversation_context_selection"
	MemoryContextSelection            ID = "memory_context_selection"
	ConversationObjectiveKind         ID = "conversation_objective_kind"
	ObjectiveAdvisory                 ID = "objective_advisory"
	ConversationResponse              ID = "conversation_response"
	RoleplayCanonExtraction           ID = "roleplay_canon_extraction"
	GroundedAnswer                    ID = "grounded_answer"
	DatabaseSchemaSelection           ID = "database_schema_selection"
	DatabaseQueryIntent               ID = "database_query_intent"
	DatabaseEvidenceGap               ID = "database_evidence_gap"
	DatabaseJoinPathSelection         ID = "database_join_path_selection"
	RepositoryEvidenceRelevance       ID = "repository_evidence_relevance"
	RepositoryGroundedReview          ID = "repository_grounded_review"
	RepositoryGroundedCorrection      ID = "repository_grounded_correction"
	WebSearchTerms                    ID = "web_search_terms"
	WebRelevance                      ID = "web_relevance"
	WebGroundedSynthesis              ID = "web_grounded_synthesis"
	WebGroundedSynthesisCorrection    ID = "web_grounded_synthesis_correction"
	WebClaimEvidenceReview            ID = "web_claim_evidence_review"
	CodingSurface                     ID = "coding_surface"
	CodingRequirements                ID = "coding_requirements"
	CodingWorkload                    ID = "coding_workload"
	CodingWorkloadReview              ID = "coding_workload_review"
	CodingTargetTree                  ID = "coding_target_tree"
	CodingArtifactHandling            ID = "coding_artifact_handling"
	CodingKnownArtifactTruth          ID = "coding_known_artifact_truth"
	CodingDeclarationArtifactBoundary ID = "coding_declaration_artifact_boundary"
	CodingArtifactCandidateSelection  ID = "coding_artifact_candidate_selection"
	CodingCapabilityRelation          ID = "coding_capability_relation"
	CodingSkillSelection              ID = "coding_skill_selection"
	CodingFragment                    ID = "coding_fragment"
	CodingFragmentRepairGuidance      ID = "coding_fragment_repair_guidance"
	CodingFragmentCorrection          ID = "coding_fragment_correction"
	CodingRepositorySearchTerm        ID = "coding_repository_search_term"
	CodingRepositoryChange            ID = "coding_repository_change_surface"
)

var registered = [...]ID{
	ConversationContextSelection,
	MemoryContextSelection,
	ConversationObjectiveKind,
	ObjectiveAdvisory,
	ConversationResponse,
	RoleplayCanonExtraction,
	GroundedAnswer,
	DatabaseSchemaSelection,
	DatabaseQueryIntent,
	DatabaseEvidenceGap,
	DatabaseJoinPathSelection,
	RepositoryEvidenceRelevance,
	RepositoryGroundedReview,
	RepositoryGroundedCorrection,
	WebSearchTerms,
	WebRelevance,
	WebGroundedSynthesis,
	WebGroundedSynthesisCorrection,
	WebClaimEvidenceReview,
	CodingSurface,
	CodingRequirements,
	CodingWorkload,
	CodingWorkloadReview,
	CodingTargetTree,
	CodingArtifactHandling,
	CodingKnownArtifactTruth,
	CodingDeclarationArtifactBoundary,
	CodingArtifactCandidateSelection,
	CodingCapabilityRelation,
	CodingSkillSelection,
	CodingFragment,
	CodingFragmentRepairGuidance,
	CodingFragmentCorrection,
	CodingRepositorySearchTerm,
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
