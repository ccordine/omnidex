package modelconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	EnvKeys     []string `json:"env_keys"`
	Options     []string `json:"options,omitempty"`
}

var Fields = []Field{
	{Key: "conversation_context_selection_model", Label: "Conversation context selection", Description: "Selects only required prior user authority IDs from one code-bounded candidate set", EnvKeys: []string{"OMNI_CONVERSATION_CONTEXT_SELECTION_MODEL"}},
	{Key: "memory_context_selection_model", Label: "Memory context selection", Description: "Selects only relevant immutable memory IDs from one code-bounded scoped candidate set", EnvKeys: []string{"OMNI_MEMORY_CONTEXT_SELECTION_MODEL"}},
	{Key: "conversation_objective_kind_model", Label: "Conversation objective kind", Description: "Classifies one exact free-form turn into one registered objective kind", EnvKeys: []string{"OMNI_CONVERSATION_OBJECTIVE_KIND_MODEL"}},
	{Key: "objective_advisory_model", Label: "Objective advisory", Description: "Returns passive non-authoritative plain-text considerations for one grounded objective", EnvKeys: []string{"OMNI_OBJECTIVE_ADVISORY_MODEL"}},
	{Key: "conversation_response_model", Label: "Conversation response", Description: "Returns one bounded answer or story response", EnvKeys: []string{"OMNI_CONVERSATION_RESPONSE_MODEL"}},
	{Key: "grounded_answer_model", Label: "Grounded answer", Description: "Synthesizes one answer from code-selected evidence IDs", EnvKeys: []string{"OMNI_GROUNDED_ANSWER_MODEL"}},
	{Key: "repository_evidence_relevance_model", Label: "Repository evidence relevance", Description: "Selects relevant IDs or none from one bounded repository evidence set", EnvKeys: []string{"OMNI_REPOSITORY_EVIDENCE_RELEVANCE_MODEL"}},
	{Key: "repository_grounded_review_model", Label: "Repository grounded review", Description: "Independently checks one answer against only its cited repository evidence", EnvKeys: []string{"OMNI_REPOSITORY_GROUNDED_REVIEW_MODEL"}},
	{Key: "repository_grounded_correction_model", Label: "Repository grounded correction", Description: "Corrects one reviewed repository answer text leaf while retaining evidence IDs", EnvKeys: []string{"OMNI_REPOSITORY_GROUNDED_CORRECTION_MODEL"}},
	{Key: "web_search_terms_model", Label: "Web search terms", Description: "Returns alternate terms for one unresolved search concept", EnvKeys: []string{"OMNI_WEB_SEARCH_TERMS_MODEL"}},
	{Key: "web_relevance_model", Label: "Web relevance", Description: "Selects relevant IDs from one bounded web candidate set", EnvKeys: []string{"OMNI_WEB_RELEVANCE_MODEL"}},
	{Key: "web_grounded_synthesis_model", Label: "Web grounded synthesis", Description: "Synthesizes bounded paragraphs from code-selected web evidence IDs", EnvKeys: []string{"OMNI_WEB_GROUNDED_SYNTHESIS_MODEL"}},
	{Key: "web_grounded_synthesis_correction_model", Label: "Web grounded synthesis correction", Description: "Corrects one exact reviewed paragraph against retained web evidence", EnvKeys: []string{"OMNI_WEB_GROUNDED_SYNTHESIS_CORRECTION_MODEL"}},
	{Key: "web_claim_evidence_review_model", Label: "Web claim-evidence review", Description: "Checks one synthesized paragraph against only its cited web evidence", EnvKeys: []string{"OMNI_WEB_CLAIM_EVIDENCE_REVIEW_MODEL"}},
	{Key: "coding_surface_model", Label: "Coding surface", Description: "Classifies only the requested delivery surface", EnvKeys: []string{"OMNI_CODING_SURFACE_MODEL"}},
	{Key: "coding_requirements_model", Label: "Coding requirements", Description: "Extracts one bounded grounded product-and-requirements aggregate from the intact application request", EnvKeys: []string{"OMNI_CODING_REQUIREMENTS_MODEL"}},
	{Key: "coding_workload_model", Label: "Coding workload", Description: "Proposes one bounded task contract per accepted application requirement and repairs only one validator-named field", EnvKeys: []string{"OMNI_CODING_WORKLOAD_MODEL"}},
	{Key: "coding_workload_review_model", Label: "Coding workload review", Description: "Independently accepts one derived task contract or names exactly one field for planner repair", EnvKeys: []string{"OMNI_CODING_WORKLOAD_REVIEW_MODEL"}},
	{Key: "coding_artifact_handling_model", Label: "Coding artifact handling", Description: "Classifies explicit artifact truth and resolves bounded path-blind artifact or declaration candidates", EnvKeys: []string{"OMNI_CODING_ARTIFACT_HANDLING_MODEL"}},
	{Key: "coding_capability_relation_model", Label: "Coding capability relation", Description: "Classifies one direct state dependency between two local needs", EnvKeys: []string{"OMNI_CODING_CAPABILITY_RELATION_MODEL"}},
	{Key: "coding_skill_selection_model", Label: "Coding skill selection", Description: "Selects one opaque validated procedure for one local need, or none", EnvKeys: []string{"OMNI_CODING_SKILL_SELECTION_MODEL"}},
	{Key: "coding_fragment_model", Label: "Coding fragment", Description: "Returns one exact path-blind function declaration from a bounded local contract", EnvKeys: []string{"OMNI_CODING_FRAGMENT_MODEL"}},
	{Key: "coding_fragment_correction_model", Label: "Coding fragment correction", Description: "Corrects one current function from one exact local diagnostic", EnvKeys: []string{"OMNI_CODING_FRAGMENT_CORRECTION_MODEL"}},
	{Key: "coding_repository_search_term_model", Label: "Repository search term", Description: "Returns one alternate term for one unresolved repository concept", EnvKeys: []string{"OMNI_CODING_REPOSITORY_SEARCH_TERM_MODEL"}},
	{Key: "coding_repository_change_surface_model", Label: "Repository change surface", Description: "Selects bounded symbol IDs for one established repository requirement", EnvKeys: []string{"OMNI_CODING_REPOSITORY_CHANGE_SURFACE_MODEL"}},
}

type Config map[string]string

func FromEnv() Config {
	out := Config{}
	for _, field := range Fields {
		if value := lookupEnv(field.EnvKeys); value != "" {
			out[field.Key] = value
		}
	}
	return out
}

func FromJSON(raw json.RawMessage) (Config, error) {
	out := Config{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return out, nil
	}
	var payload map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("model config must be a JSON object: %w", err)
	}
	if err := requireEOF(decoder, "model config"); err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, fmt.Errorf("model config must be a JSON object, received null")
	}
	allowed := modelConfigKeys()
	for key, rawValue := range payload {
		if _, ok := allowed[key]; !ok {
			return nil, fmt.Errorf("model config contains unsupported field %q", key)
		}
		var value string
		if err := json.Unmarshal(rawValue, &value); err != nil {
			return nil, fmt.Errorf("model config field %q must be a string: %w", key, err)
		}
		if value = strings.TrimSpace(value); value != "" {
			out[key] = value
		}
	}
	return out, nil
}

func FromSettingsJSON(raw json.RawMessage) (Config, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return Config{}, nil
	}
	var settings map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if err := decoder.Decode(&settings); err != nil {
		return nil, fmt.Errorf("project settings must be a JSON object: %w", err)
	}
	if err := requireEOF(decoder, "project settings"); err != nil {
		return nil, err
	}
	if settings == nil {
		return nil, fmt.Errorf("project settings must be a JSON object, received null")
	}
	if nested, ok := settings["model_config"]; ok {
		return FromJSON(nested)
	}
	return Config{}, nil
}

func modelConfigKeys() map[string]struct{} {
	keys := make(map[string]struct{}, len(Fields))
	for _, field := range Fields {
		keys[field.Key] = struct{}{}
	}
	return keys
}

func requireEOF(decoder *json.Decoder, label string) error {
	var trailing any
	err := decoder.Decode(&trailing)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("%s contains trailing JSON", label)
	}
	return fmt.Errorf("%s contains trailing data: %w", label, err)
}

func Merge(layers ...Config) Config {
	out := Config{}
	for _, layer := range layers {
		for key, value := range layer {
			if strings.TrimSpace(value) != "" {
				out[key] = strings.TrimSpace(value)
			}
		}
	}
	return out
}

func (c Config) Get(key string) string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c[key])
}

func (c Config) ModelNames() []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, field := range Fields {
		value := c.Get(field.Key)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func (c Config) ToMap() map[string]string {
	out := map[string]string{}
	for key, value := range c {
		if strings.TrimSpace(value) != "" {
			out[key] = strings.TrimSpace(value)
		}
	}
	return out
}

func (c Config) FieldList(envValues map[string]string) []map[string]any {
	if envValues == nil {
		envValues = map[string]string{}
	}
	items := make([]map[string]any, 0, len(Fields))
	for _, field := range Fields {
		value := c.Get(field.Key)
		if value == "" {
			value = lookupMap(envValues, field.EnvKeys)
		}
		if value == "" {
			value = lookupEnv(field.EnvKeys)
		}
		items = append(items, map[string]any{
			"key":         field.Key,
			"label":       field.Label,
			"description": field.Description,
			"env_keys":    field.EnvKeys,
			"options":     field.Options,
			"value":       value,
		})
	}
	return items
}

func lookupEnv(keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

func lookupMap(values map[string]string, keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}
