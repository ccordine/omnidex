package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func (s *Server) envModelConfig() modelconfig.Config {
	return modelconfig.FromEnv()
}

func (s *Server) projectModelConfig(project model.Project) (modelconfig.Config, error) {
	return modelconfig.FromSettingsJSON(project.Settings)
}

func (s *Server) resolveModelConfig(project model.Project) (modelconfig.Config, string, error) {
	env := s.envModelConfig()
	projectCfg, err := s.projectModelConfig(project)
	if err != nil {
		return nil, "", fmt.Errorf("parse project model config: %w", err)
	}
	resolved := modelconfig.Merge(env, projectCfg)
	source := "env"
	if len(projectCfg) > 0 {
		source = "project"
	}
	return resolved, source, nil
}

func mergeProjectModelConfig(settings json.RawMessage, modelConfig json.RawMessage) (json.RawMessage, error) {
	var root map[string]json.RawMessage
	if len(settings) > 0 {
		if err := json.Unmarshal(settings, &root); err != nil {
			return nil, err
		}
	}
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	if len(modelConfig) > 0 {
		parsed, err := modelconfig.FromJSON(modelConfig)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(parsed.ToMap())
		if err != nil {
			return nil, err
		}
		root["model_config"] = encoded
	}
	out, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) resolvedModelsForProject(ctx context.Context, projectID int64) (map[string]any, error) {
	if s.repo == nil || projectID <= 0 {
		resolved, source, err := s.resolveModelConfig(model.Project{})
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"resolved": resolved.ToMap(),
			"source":   source,
			"fields":   resolved.FieldList(map[string]string{}),
		}, nil
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	resolved, source, err := s.resolveModelConfig(project)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"resolved": resolved.ToMap(),
		"source":   source,
		"fields":   resolved.FieldList(map[string]string{}),
	}, nil
}

func modelConfigPatchFromRequest(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	config, err := modelconfig.FromJSON(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid model_config: %w", err)
	}
	out, err := json.Marshal(config.ToMap())
	if err != nil {
		return nil, err
	}
	return out, nil
}
