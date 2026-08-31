package station

import "fmt"

// ID names one bounded semantic transformation. It is model routing data, not
// a persona, tool owner, workflow, or completion authority.
type ID string

const (
	ContextRelevance                              ID = "context_relevance"
	ContextMinification                           ID = "context_minification"
	ConversationObjectiveKind                     ID = "conversation_objective_kind"
	ConversationResponse                          ID = "conversation_response"
	RoleplayCanonExtraction                       ID = "roleplay_canon_extraction"
	RoleplayOngoingAction                         ID = "roleplay_ongoing_action"
	GroundedAnswer                                ID = "grounded_answer"
	DatabaseSchemaSelection                       ID = "database_schema_selection"
	DatabaseQueryIntent                           ID = "database_query_intent"
	DatabaseJoinPathSelection                     ID = "database_join_path_selection"
	WebRelevance                                  ID = "web_relevance"
	WebGroundedSynthesis                          ID = "web_grounded_synthesis"
	CodingSurface                                 ID = "coding_surface"
	CodingRequirements                            ID = "coding_requirements"
	CodingProjectStackConstraint                  ID = "coding_project_stack_constraint"
	CodingArtifactHandling                        ID = "coding_artifact_handling"
	CodingCapabilityRelation                      ID = "coding_capability_relation"
	CodingRuntimeCapabilityNecessity              ID = "coding_runtime_capability_necessity"
	CodingFragment                                ID = "coding_fragment"
	CodingFragmentRepairGuidance                  ID = "coding_fragment_repair_guidance"
	CodingFragmentCorrection                      ID = "coding_fragment_correction"
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
	DatabaseJoinPathSelection,
	WebRelevance,
	WebGroundedSynthesis,
	CodingSurface,
	CodingRequirements,
	CodingProjectStackConstraint,
	CodingArtifactHandling,
	CodingCapabilityRelation,
	CodingRuntimeCapabilityNecessity,
	CodingFragment,
	CodingFragmentRepairGuidance,
	CodingFragmentCorrection,
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
