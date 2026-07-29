package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func modelRoutingFromJobMetadata(metadata json.RawMessage, base ModelRouting) (ModelRouting, error) {
	if len(metadata) == 0 {
		return base, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return ModelRouting{}, fmt.Errorf("parse job model routing metadata: %w", err)
	}
	if err := validateExplicitJobModels(payload); err != nil {
		return ModelRouting{}, err
	}
	cfg := modelconfig.Config{}
	if raw, ok := payload["model_config"]; ok {
		bytes, err := json.Marshal(raw)
		if err != nil {
			return ModelRouting{}, fmt.Errorf("encode job model config: %w", err)
		}
		cfg, err = modelconfig.FromJSON(bytes)
		if err != nil {
			return ModelRouting{}, fmt.Errorf("parse job model config: %w", err)
		}
	}
	if len(cfg) == 0 {
		return base, nil
	}
	baseRouting := modelconfig.Routing{
		Default:    base.Default,
		Fast:       base.Fast,
		Glue:       base.Glue,
		Reasoning:  base.Reasoning,
		Tagging:    base.Tagging,
		Plan:       base.Plan,
		Analyze:    base.Analyze,
		Response:   base.Response,
		Search:     base.Search,
		Memory:     base.Memory,
		Specialist: base.Specialist,
	}
	applied := modelconfig.Apply(baseRouting, cfg)
	return ModelRouting{
		Default:    applied.Default,
		Fast:       applied.Fast,
		Glue:       applied.Glue,
		Reasoning:  applied.Reasoning,
		Tagging:    applied.Tagging,
		Plan:       applied.Plan,
		Analyze:    applied.Analyze,
		Response:   applied.Response,
		Search:     applied.Search,
		Memory:     applied.Memory,
		Specialist: applied.Specialist,
	}, nil
}

func validateExplicitJobModels(payload map[string]any) error {
	for _, key := range []string{
		"model_plan",
		"model_analyze",
		"model_response",
		"model_search",
		"model_tagger",
		"model_verify",
		"model_memory",
		"model_execute",
		"model_source_review",
		"model_source",
	} {
		value, exists := payload[key]
		if !exists {
			continue
		}
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return fmt.Errorf("job model routing metadata %s must be a non-empty string", key)
		}
	}
	return nil
}

func (s *Service) v3SpecialistModel(job model.Job, routing ModelRouting, skillID, defaultRoleID, fallback string) string {
	if key := v3SkillModelOverrideKey(skillID); key != "" {
		if explicit := metadataModel(job, key, ""); explicit != "" {
			return explicit
		}
	}
	return s.skillPreferredModel(skillID, specialistModel(job, defaultRoleID, fallback, routing), routing)
}

func v3SkillModelOverrideKey(skillID string) string {
	switch strings.TrimSpace(skillID) {
	case "prompt_interpreter":
		return "model_tagger"
	case "executive_planner":
		return "model_plan"
	case "workspace_researcher", "analysis_specialist":
		return "model_analyze"
	case "subtask_executor":
		return "model_execute"
	case "coding_fragment":
		return "model_fragment"
	case "response_composer":
		return "model_response"
	case "verifier":
		return "model_verify"
	case "web_researcher":
		return "model_search"
	case "memory_reviewer":
		return "model_memory"
	default:
		return ""
	}
}

func modelConfigSource(metadata json.RawMessage) string {
	source := strings.TrimSpace(metadataString(metadata, "model_config_source"))
	if source == "" {
		return "env"
	}
	return source
}
