package modelconfig

import (
	"os"

	"github.com/gryph/omnidex/internal/station"
)

// Field is one externally configurable model-routing leaf.
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	EnvKeys     []string `json:"env_keys"`
	Options     []string `json:"options,omitempty"`
}

type fieldDefinition struct {
	Key              string
	Label            string
	Description      string
	EnvironmentKey   string
	Options          []string
	Stations         []station.ID
	RoleplaySemantic bool
}

// fieldRegistry is the sole model-field, environment, and station projection
// authority. Every other representation is derived mechanically from it.
var fieldRegistry = []fieldDefinition{
	{Key: "context_relevance_model", Label: "Context relevance", Description: "Judges one code-known context candidate against one exact instruction", EnvironmentKey: "OMNI_CONTEXT_RELEVANCE_MODEL", Stations: []station.ID{station.ContextRelevance}},
	{Key: "context_minification_model", Label: "Context minification", Description: "Reduces selected exact authorities into one bounded minimal-context leaf", EnvironmentKey: "OMNI_CONTEXT_MINIFICATION_MODEL", Stations: []station.ID{station.ContextMinification}},
	{Key: "conversation_objective_kind_model", Label: "Conversation objective kind", Description: "Classifies one exact free-form turn into one registered objective kind", EnvironmentKey: "OMNI_CONVERSATION_OBJECTIVE_KIND_MODEL", Stations: []station.ID{station.ConversationObjectiveKind}},
	{Key: "conversation_response_model", Label: "Conversation response", Description: "Returns one bounded answer or story response", EnvironmentKey: "OMNI_CONVERSATION_RESPONSE_MODEL", Stations: []station.ID{station.ConversationResponse}},
	{Key: "roleplay_semantic_model", Label: "Roleplay semantics", Description: "Resolves bounded fictional canon, ongoing-action, and context leaves for roleplay turns", EnvironmentKey: "OMNI_ROLEPLAY_SEMANTIC_MODEL", RoleplaySemantic: true},
	{Key: "grounded_answer_model", Label: "Grounded answer", Description: "Returns one bounded untrusted paragraph inventory, one evidence-support relation, or one paragraph-authorization relation per repository-grounding call", EnvironmentKey: "OMNI_GROUNDED_ANSWER_MODEL", Stations: []station.ID{station.GroundedAnswer}},
	{Key: "database_schema_selection_model", Label: "Database schema selection", Description: "Selects the model for separate bounded relation-responsibility inventory, candidate-necessity, and registered-relation resolution calls", EnvironmentKey: "OMNI_DATABASE_SCHEMA_SELECTION_MODEL", Stations: []station.ID{station.DatabaseSchemaSelection}},
	{Key: "database_query_intent_model", Label: "Database query intent", Description: "Expresses one exact evidence need as a typed relational intent over known opaque schema IDs", EnvironmentKey: "OMNI_DATABASE_QUERY_INTENT_MODEL", Stations: []station.ID{station.DatabaseQueryIntent}},
	{Key: "database_join_path_selection_model", Label: "Database join path selection", Description: "Selects one opaque current foreign-key path for one named semantic relationship ambiguity", EnvironmentKey: "OMNI_DATABASE_JOIN_PATH_SELECTION_MODEL", Stations: []station.ID{station.DatabaseJoinPathSelection}},
	{Key: "web_relevance_model", Label: "Web relevance", Description: "Judges one code-known web evidence candidate against one exact question", EnvironmentKey: "OMNI_WEB_RELEVANCE_MODEL", Stations: []station.ID{station.WebRelevance}},
	{Key: "coding_surface_model", Label: "Coding surface", Description: "Classifies only the requested delivery surface", EnvironmentKey: "OMNI_CODING_SURFACE_MODEL", Stations: []station.ID{station.CodingSurface}},
	{Key: "coding_requirements_model", Label: "Coding intent", Description: "Returns one bounded product or stack value, one untrusted requirement inventory, or one candidate-bound sieve result per coding-intent call", EnvironmentKey: "OMNI_CODING_REQUIREMENTS_MODEL", Stations: []station.ID{station.CodingRequirements, station.CodingProjectStackConstraint}},
	{Key: "coding_requirement_result_relation_model", Label: "Coding requirement result relation", Description: "Answers one candidate-bound derived-value or determining-relation presence question", EnvironmentKey: "OMNI_CODING_REQUIREMENT_RESULT_RELATION_MODEL", Stations: []station.ID{station.CodingRequirementResultRelation}},
	{Key: "coding_artifact_handling_model", Label: "Coding artifact handling", Description: "Classifies explicit artifact truth and resolves bounded path-blind artifact or declaration candidates", EnvironmentKey: "OMNI_CODING_ARTIFACT_HANDLING_MODEL", Stations: []station.ID{station.CodingArtifactHandling}},
	{Key: "coding_capability_relation_model", Label: "Coding capability relation", Description: "Resolves one pairwise direct-dependency relation per call", EnvironmentKey: "OMNI_CODING_CAPABILITY_RELATION_MODEL", Stations: []station.ID{station.CodingCapabilityRelation}},
	{Key: "coding_fragment_model", Label: "Coding fragment", Description: "Returns one exact path-blind function declaration from a bounded local contract", EnvironmentKey: "OMNI_CODING_FRAGMENT_MODEL", Stations: []station.ID{station.CodingFragment}},
	{Key: "coding_fragment_repair_guidance_model", Label: "Coding repair guidance", Description: "Diagnoses one exact local validation failure into one self-contained source-repair instruction", EnvironmentKey: "OMNI_CODING_FRAGMENT_REPAIR_GUIDANCE_MODEL", Stations: []station.ID{station.CodingFragmentRepairGuidance}},
	{Key: "coding_fragment_correction_model", Label: "Coding fragment correction", Description: "Executes one repair instruction against only its exact mutable source block", EnvironmentKey: "OMNI_CODING_FRAGMENT_CORRECTION_MODEL", Stations: []station.ID{station.CodingFragmentCorrection}},
}

// RegisteredFields returns an isolated presentation projection of the registry.
func RegisteredFields() []Field {
	fields := make([]Field, len(fieldRegistry))
	for index, definition := range fieldRegistry {
		fields[index] = Field{
			Key: definition.Key, Label: definition.Label, Description: definition.Description,
			EnvKeys: []string{definition.EnvironmentKey},
			Options: append([]string(nil), definition.Options...),
		}
	}
	return fields
}

func definitionForKey(key string) (fieldDefinition, bool) {
	for _, definition := range fieldRegistry {
		if definition.Key == key {
			return definition, true
		}
	}
	return fieldDefinition{}, false
}

// LoadEnvironment freezes the process environment once through the canonical
// registry. Runtime consumers never read environment variables themselves.
func LoadEnvironment() Authority {
	config := Config{}
	for _, definition := range fieldRegistry {
		value, configured := os.LookupEnv(definition.EnvironmentKey)
		if !configured {
			continue
		}
		config[definition.Key] = value
	}
	return Authority{config: config}
}
