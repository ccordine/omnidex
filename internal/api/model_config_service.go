package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/llmprovider/catalog"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/ollama"
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

func (s *Server) ensureOllamaModels(ctx context.Context, cfg modelconfig.Config) ([]string, error) {
	models := cfg.ModelNames()
	if len(models) == 0 {
		return nil, nil
	}
	definition, ok := catalog.Lookup(s.defaultProvider)
	if !ok {
		return nil, fmt.Errorf("cannot prepare models for unsupported LLM provider %q", s.defaultProvider)
	}
	if definition.ID != "ollama" {
		return nil, nil
	}
	client := s.ollamaClientWithTimeout(30 * time.Second)
	pulled, err := client.EnsureModels(ctx, models)
	if err != nil && isOllamaConnectivityError(err) {
		endpoint := s.refreshOllamaEndpoint(ctx)
		client = ollama.New(endpoint, "", "", 30*time.Second, s.providerConfig.InferenceContextTokens)
		return client.EnsureModels(ctx, models)
	}
	return pulled, err
}

func (s *Server) modelConfigJobMetadata(ctx context.Context, project model.Project, card ScrumCard) (map[string]any, []string, error) {
	resolved, source, err := s.resolveModelConfig(project, card)
	if err != nil {
		return nil, nil, err
	}
	pulled, err := s.ensureOllamaModels(ctx, resolved)
	if err != nil {
		return nil, pulled, err
	}
	return map[string]any{
		"model_config":        resolved.ToMap(),
		"model_config_source": source,
	}, pulled, nil
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

func (s *Server) enrichJobMetadata(ctx context.Context, metadata []byte, card ScrumCard) ([]byte, []string, error) {
	if len(metadata) == 0 {
		metadata = []byte(`{}`)
	}
	var payload map[string]any
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return metadata, nil, fmt.Errorf("metadata must be a JSON object")
	}
	if payload == nil {
		payload = map[string]any{}
	}
	project := model.Project{}
	projectID := metadataInt64(payload, "project_id")
	if projectID > 0 && s.repo != nil {
		loaded, err := s.repo.GetProject(ctx, projectID)
		if err != nil {
			return metadata, nil, fmt.Errorf("load metadata project %d: %w", projectID, err)
		}
		project = loaded
	}
	if card.ID == "" {
		cardID := metadataString(payload, "scrum_card_id")
		if cardID != "" && projectID > 0 && s.repo != nil {
			dbCard, err := s.repo.GetScrumCard(ctx, projectID, cardID)
			if err != nil {
				return metadata, nil, fmt.Errorf("load metadata Scrum card %q: %w", cardID, err)
			}
			card, err = dbScrumCardToAPI(dbCard)
			if err != nil {
				return metadata, nil, fmt.Errorf("decode metadata Scrum card %q: %w", cardID, err)
			}
		}
	}

	var instance agentconfig.Config
	if raw, ok := payload["instance_agent_config"]; ok && raw != nil {
		bytes, err := json.Marshal(raw)
		if err != nil {
			return metadata, nil, fmt.Errorf("encode instance_agent_config: %w", err)
		}
		instance, err = agentconfig.FromJSON(bytes)
		if err != nil {
			return metadata, nil, fmt.Errorf("parse instance_agent_config: %w", err)
		}
	}

	var pulled []string
	if _, ok := payload["model_config"]; !ok {
		extra, modelPulled, err := s.modelConfigJobMetadata(ctx, project, card)
		if err != nil {
			return metadata, modelPulled, err
		}
		for key, value := range extra {
			payload[key] = value
		}
		pulled = modelPulled
	}
	if _, ok := payload["agent_config"]; !ok {
		if generalWebChatWithoutWorkspace(payload) {
			payload["agent_config"] = map[string]any{"agent_system": agentconfig.SystemOmnidex}
			payload["agent_config_source"] = "general_chat"
		} else {
			extra, err := s.agentConfigJobMetadata(ctx, project, card, instance)
			if err != nil {
				return metadata, pulled, err
			}
			for key, value := range extra {
				payload[key] = value
			}
		}
	}
	if len(pulled) > 0 {
		payload["models_pulled"] = pulled
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return metadata, pulled, err
	}
	if _, err := agentconfig.FromJobMetadata(out); err != nil {
		return metadata, pulled, err
	}
	return out, pulled, nil
}

func metadataInt64(payload map[string]any, key string) int64 {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return 0
	}
	switch value := raw.(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case json.Number:
		parsed, _ := value.Int64()
		return parsed
	default:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
		return parsed
	}
}

func metadataString(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func generalWebChatWithoutWorkspace(payload map[string]any) bool {
	if !webChatJobMetadata(payload) {
		return false
	}
	if metadataInt64(payload, "project_id") > 0 {
		return false
	}
	for _, key := range []string{"client_cwd", "project_directory", "workspace"} {
		if metadataString(payload, key) != "" {
			return false
		}
	}
	return true
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
