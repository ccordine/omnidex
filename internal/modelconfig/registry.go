package modelconfig

import (
	"fmt"
	"os"
	"strings"

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
	Field
	Stations         []station.ID
	RoleplaySemantic bool
}

// fieldRegistry is the sole model-field, environment, and station projection
// authority. Every other representation is derived mechanically from it.
var fieldRegistry = []fieldDefinition{
	{Field: Field{Key: "context_relevance_model", Label: "Context relevance", Description: "Judges one code-known context candidate against one exact instruction", EnvKeys: []string{"OMNI_CONTEXT_RELEVANCE_MODEL"}}, Stations: []station.ID{station.ContextRelevance}},
	{Field: Field{Key: "context_minification_model", Label: "Context minification", Description: "Reduces selected exact authorities into one bounded minimal-context leaf", EnvKeys: []string{"OMNI_CONTEXT_MINIFICATION_MODEL"}}, Stations: []station.ID{station.ContextMinification}},
	{Field: Field{Key: "conversation_objective_kind_model", Label: "Conversation objective kind", Description: "Classifies one exact free-form turn into one registered objective kind", EnvKeys: []string{"OMNI_CONVERSATION_OBJECTIVE_KIND_MODEL"}}, Stations: []station.ID{station.ConversationObjectiveKind}},
	{Field: Field{Key: "conversation_response_model", Label: "Conversation response", Description: "Returns one bounded answer or story response", EnvKeys: []string{"OMNI_CONVERSATION_RESPONSE_MODEL"}}, Stations: []station.ID{station.ConversationResponse}},
	{Field: Field{Key: "roleplay_semantic_model", Label: "Roleplay semantics", Description: "Resolves bounded fictional canon, ongoing-action, and context leaves for roleplay turns", EnvKeys: []string{"OMNI_ROLEPLAY_SEMANTIC_MODEL"}}, RoleplaySemantic: true},
	{Field: Field{Key: "grounded_answer_model", Label: "Grounded answer", Description: "Returns one bounded untrusted paragraph inventory, one evidence-support relation, or one paragraph-authorization relation per repository-grounding call", EnvKeys: []string{"OMNI_GROUNDED_ANSWER_MODEL"}}, Stations: []station.ID{station.GroundedAnswer}},
	{Field: Field{Key: "database_schema_selection_model", Label: "Database schema selection", Description: "Selects the model for separate bounded relation-responsibility inventory, candidate-necessity, and registered-relation resolution calls", EnvKeys: []string{"OMNI_DATABASE_SCHEMA_SELECTION_MODEL"}}, Stations: []station.ID{station.DatabaseSchemaSelection}},
	{Field: Field{Key: "database_query_intent_model", Label: "Database query intent", Description: "Expresses one exact evidence need as a typed relational intent over known opaque schema IDs", EnvKeys: []string{"OMNI_DATABASE_QUERY_INTENT_MODEL"}}, Stations: []station.ID{station.DatabaseQueryIntent}},
	{Field: Field{Key: "database_join_path_selection_model", Label: "Database join path selection", Description: "Selects one opaque current foreign-key path for one named semantic relationship ambiguity", EnvKeys: []string{"OMNI_DATABASE_JOIN_PATH_SELECTION_MODEL"}}, Stations: []station.ID{station.DatabaseJoinPathSelection}},
	{Field: Field{Key: "web_relevance_model", Label: "Web relevance", Description: "Judges one code-known web evidence candidate against one exact question", EnvKeys: []string{"OMNI_WEB_RELEVANCE_MODEL"}}, Stations: []station.ID{station.WebRelevance}},
	{Field: Field{Key: "coding_surface_model", Label: "Coding surface", Description: "Classifies only the requested delivery surface", EnvKeys: []string{"OMNI_CODING_SURFACE_MODEL"}}, Stations: []station.ID{station.CodingSurface}},
	{Field: Field{Key: "coding_requirements_model", Label: "Coding intent", Description: "Returns one bounded product or stack value, one untrusted context-question or requirement inventory, or one candidate-bound sieve result per coding-intent call", EnvKeys: []string{"OMNI_CODING_REQUIREMENTS_MODEL"}}, Stations: []station.ID{station.CodingRequirements, station.CodingProjectStackConstraint}},
	{Field: Field{Key: "coding_artifact_handling_model", Label: "Coding artifact handling", Description: "Classifies explicit artifact truth and resolves bounded path-blind artifact or declaration candidates", EnvKeys: []string{"OMNI_CODING_ARTIFACT_HANDLING_MODEL"}}, Stations: []station.ID{station.CodingArtifactHandling}},
	{Field: Field{Key: "coding_capability_relation_model", Label: "Coding capability relation", Description: "Resolves one pairwise direct-dependency relation or one candidate-bound runtime-necessity relation per call", EnvKeys: []string{"OMNI_CODING_CAPABILITY_RELATION_MODEL"}}, Stations: []station.ID{station.CodingCapabilityRelation, station.CodingRuntimeCapabilityNecessity}},
	{Field: Field{Key: "coding_fragment_model", Label: "Coding fragment", Description: "Returns one exact path-blind function declaration from a bounded local contract", EnvKeys: []string{"OMNI_CODING_FRAGMENT_MODEL"}}, Stations: []station.ID{station.CodingFragment}},
	{Field: Field{Key: "coding_fragment_repair_guidance_model", Label: "Coding repair guidance", Description: "Diagnoses one exact local validation failure into one self-contained source-repair instruction", EnvKeys: []string{"OMNI_CODING_FRAGMENT_REPAIR_GUIDANCE_MODEL"}}, Stations: []station.ID{station.CodingFragmentRepairGuidance}},
	{Field: Field{Key: "coding_fragment_correction_model", Label: "Coding fragment correction", Description: "Executes one repair instruction against only its exact mutable source block", EnvKeys: []string{"OMNI_CODING_FRAGMENT_CORRECTION_MODEL"}}, Stations: []station.ID{station.CodingFragmentCorrection}},
}

// RegisteredFields returns an isolated presentation projection of the registry.
func RegisteredFields() []Field {
	fields := make([]Field, len(fieldRegistry))
	for index, definition := range fieldRegistry {
		field := definition.Field
		field.EnvKeys = append([]string(nil), field.EnvKeys...)
		field.Options = append([]string(nil), field.Options...)
		fields[index] = field
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
func LoadEnvironment() (Authority, error) {
	config := Config{}
	for _, definition := range fieldRegistry {
		if len(definition.EnvKeys) != 1 || strings.TrimSpace(definition.EnvKeys[0]) == "" {
			return Authority{}, fmt.Errorf("model field %q must register one environment key", definition.Key)
		}
		value, configured := os.LookupEnv(definition.EnvKeys[0])
		if !configured {
			continue
		}
		config[definition.Key] = value
	}
	return Freeze(config)
}
