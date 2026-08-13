package station

import "fmt"

// ID names one bounded semantic transformation. It is model routing data, not
// a persona, tool owner, workflow, or completion authority.
type ID string

const (
	ConversationContextSelection      ID = "conversation_context_selection"
	MemoryContextSelection            ID = "memory_context_selection"
	ConversationObjectiveKind         ID = "conversation_objective_kind"
	ConversationResponse              ID = "conversation_response"
	GroundedAnswer                    ID = "grounded_answer"
	RepositoryEvidenceRelevance       ID = "repository_evidence_relevance"
	RepositoryGroundedReview          ID = "repository_grounded_review"
	RepositoryGroundedCorrection      ID = "repository_grounded_correction"
	WebSearchTerms                    ID = "web_search_terms"
	WebRelevance                      ID = "web_relevance"
	WebGroundedSynthesis              ID = "web_grounded_synthesis"
	WebGroundedSynthesisCorrection    ID = "web_grounded_synthesis_correction"
	WebClaimEvidenceReview            ID = "web_claim_evidence_review"
	CodingSurface                     ID = "coding_surface"
	CodingProductIdentity             ID = "coding_product_identity"
	CodingRequirementPartition        ID = "coding_requirement_partition"
	CodingArtifactHandling            ID = "coding_artifact_handling"
	CodingKnownArtifactTruth          ID = "coding_known_artifact_truth"
	CodingDeclarationArtifactBoundary ID = "coding_declaration_artifact_boundary"
	CodingArtifactCandidateSelection  ID = "coding_artifact_candidate_selection"
	CodingCapabilityRelation          ID = "coding_capability_relation"
	CodingSkillSelection              ID = "coding_skill_selection"
	CodingFragment                    ID = "coding_fragment"
	CodingFragmentCorrection          ID = "coding_fragment_correction"
	CodingRepositorySearchTerm        ID = "coding_repository_search_term"
	CodingRepositoryChange            ID = "coding_repository_change_surface"
)

var registered = [...]ID{
	ConversationContextSelection,
	MemoryContextSelection,
	ConversationObjectiveKind,
	ConversationResponse,
	GroundedAnswer,
	RepositoryEvidenceRelevance,
	RepositoryGroundedReview,
	RepositoryGroundedCorrection,
	WebSearchTerms,
	WebRelevance,
	WebGroundedSynthesis,
	WebGroundedSynthesisCorrection,
	WebClaimEvidenceReview,
	CodingSurface,
	CodingProductIdentity,
	CodingRequirementPartition,
	CodingArtifactHandling,
	CodingKnownArtifactTruth,
	CodingDeclarationArtifactBoundary,
	CodingArtifactCandidateSelection,
	CodingCapabilityRelation,
	CodingSkillSelection,
	CodingFragment,
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
