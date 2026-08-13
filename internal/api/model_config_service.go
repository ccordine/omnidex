package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

func (s *Server) envModelConfig() (modelconfig.Config, error) {
	path, err := resolveEnvFilePath()
	if err != nil {
		return nil, fmt.Errorf("resolve model environment file: %w", err)
	}
	values, err := readEnvFile(path)
	if err != nil {
		return nil, fmt.Errorf("read model environment file: %w", err)
	}
	if err := modelconfig.ValidateEnvironmentValues(values); err != nil {
		return nil, fmt.Errorf("model environment file: %w", err)
	}
	if err := modelconfig.ValidateCurrentEnvironment(); err != nil {
		return nil, fmt.Errorf("process model environment: %w", err)
	}
	fileConfig := modelconfig.Config{}
	for _, field := range modelconfig.Fields {
		if value := lookupEnvFileValue(values, field.EnvKeys); value != "" {
			fileConfig[field.Key] = value
		}
	}
	return modelconfig.Merge(fileConfig, modelconfig.FromEnv()), nil
}

func lookupEnvFileValue(values map[string]string, keys []string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values[key]); value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) projectModelConfig(project model.Project) (modelconfig.Config, error) {
	return modelconfig.FromSettingsJSON(project.Settings)
}

func (s *Server) cardModelConfig(card ScrumCard) (modelconfig.Config, error) {
	if len(card.ModelConfig) == 0 {
		return modelconfig.Config{}, nil
	}
	return modelconfig.FromJSON(card.ModelConfig)
}

func (s *Server) resolveModelConfig(project model.Project, card ScrumCard) (modelconfig.Config, string, error) {
	env, err := s.envModelConfig()
	if err != nil {
		return nil, "", err
	}
	projectCfg, err := s.projectModelConfig(project)
	if err != nil {
		return nil, "", fmt.Errorf("parse project model config: %w", err)
	}
	cardCfg, err := s.cardModelConfig(card)
	if err != nil {
		return nil, "", fmt.Errorf("parse Scrum card model config: %w", err)
	}
	resolved := modelconfig.Merge(env, projectCfg, cardCfg)
	source := "env"
	if len(projectCfg) > 0 {
		source = "project"
	}
	if len(cardCfg) > 0 {
		source = "card"
	}
	return resolved, source, nil
}

func (s *Server) modelConfigJobMetadata(_ context.Context, project model.Project, card ScrumCard) (map[string]any, []string, error) {
	resolved, _, err := s.resolveModelConfig(project, card)
	if err != nil {
		return nil, nil, err
	}
	return map[string]any{
		"model_config": resolved.ToMap(),
	}, nil, nil
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

func (s *Server) handleResolvedModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	env, err := s.envModelConfig()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	projectIDText := strings.TrimSpace(r.URL.Query().Get("project_id"))
	projectID := int64(0)
	if projectIDText != "" {
		var err error
		projectID, err = strconv.ParseInt(projectIDText, 10, 64)
		if err != nil || projectID <= 0 {
			writeError(w, http.StatusBadRequest, "invalid project_id")
			return
		}
	}
	cardID := strings.TrimSpace(r.URL.Query().Get("card_id"))
	card := ScrumCard{}
	if cardID != "" {
		if s.repo == nil || projectID <= 0 {
			writeError(w, http.StatusBadRequest, "card_id requires a project_id and PostgreSQL repository")
			return
		}
		dbCard, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		card, err = dbScrumCardToAPI(dbCard)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if projectID > 0 {
		resolved, err := s.resolvedModelsForProjectCard(r.Context(), projectID, card)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"env_defaults": env.ToMap(),
			"fields":       env.FieldList(map[string]string{}),
			"resolved":     resolved,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"env_defaults": env.ToMap(),
		"fields":       env.FieldList(map[string]string{}),
	})
}

func (s *Server) resolvedModelsForProjectCard(ctx context.Context, projectID int64, card ScrumCard) (map[string]any, error) {
	if s.repo == nil || projectID <= 0 {
		resolved, source, err := s.resolveModelConfig(model.Project{}, card)
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
	resolved, source, err := s.resolveModelConfig(project, card)
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
