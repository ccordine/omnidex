package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) defaultAgentConfig(ctx context.Context) (agentconfig.Config, error) {
	cfg, _, err := s.resolveAgentConfig(ctx, model.Project{}, ScrumCard{})
	return cfg, err
}

func (s *Server) workspaceAgentConfig(ctx context.Context) (agentconfig.Config, error) {
	if s.repo == nil {
		return agentconfig.Config{}, nil
	}
	stored, err := s.repo.GetWorkspaceAgentConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("load workspace agent configuration: %w", err)
	}
	cfg, err := agentconfig.FromStringMap(stored)
	if err != nil {
		return nil, fmt.Errorf("parse workspace agent configuration: %w", err)
	}
	return cfg, nil
}

func (s *Server) projectAgentConfig(project model.Project) (agentconfig.Config, error) {
	cfg, err := agentconfig.FromSettingsJSON(project.Settings)
	if err != nil {
		return nil, fmt.Errorf("parse project agent configuration: %w", err)
	}
	return cfg, nil
}

func (s *Server) cardAgentConfig(card ScrumCard) (agentconfig.Config, error) {
	if len(card.AgentConfig) == 0 {
		return agentconfig.Config{}, nil
	}
	cfg, err := agentconfig.FromJSON(card.AgentConfig)
	if err != nil {
		return nil, fmt.Errorf("parse Scrum card agent configuration: %w", err)
	}
	return cfg, nil
}

// resolveAgentConfig merges: env → workspace (global DB) → project → card → instance.
func (s *Server) resolveAgentConfig(ctx context.Context, project model.Project, card ScrumCard, instance ...agentconfig.Config) (agentconfig.Config, string, error) {
	workspace, err := s.workspaceAgentConfig(ctx)
	if err != nil {
		return nil, "", err
	}
	projectCfg, err := s.projectAgentConfig(project)
	if err != nil {
		return nil, "", err
	}
	cardCfg, err := s.cardAgentConfig(card)
	if err != nil {
		return nil, "", err
	}
	processEnv, err := agentconfig.FromEnv()
	if err != nil {
		return nil, "", err
	}
	stack := agentconfig.Stack{
		Workspace:  workspace,
		Project:    projectCfg,
		Card:       cardCfg,
		ProcessEnv: processEnv,
	}
	path, err := resolveEnvFilePath()
	if err != nil {
		return nil, "", fmt.Errorf("resolve agent environment file: %w", err)
	}
	values, err := readEnvFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read agent environment file: %w", err)
	}
	stack.EnvFile, err = agentconfig.FromEnvFileValues(values)
	if err != nil {
		return nil, "", err
	}
	if len(instance) > 0 && len(instance[0]) > 0 {
		if err := agentconfig.Validate(instance[0]); err != nil {
			return nil, "", fmt.Errorf("instance agent configuration: %w", err)
		}
		stack.Instance = instance[0]
	}
	return stack.Resolve()
}

func (s *Server) agentConfigJobMetadata(ctx context.Context, project model.Project, card ScrumCard, instance ...agentconfig.Config) (map[string]any, error) {
	resolved, source, err := s.resolveAgentConfig(ctx, project, card, instance...)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"agent_config":        resolved.ToMap(),
		"agent_config_source": source,
	}
	if resolved.IsExternal() {
		payload["external_agents_used"] = []string{resolved.ExternalAgentName()}
	}
	return payload, nil
}

func mergeProjectAgentConfig(settings json.RawMessage, agentConfig json.RawMessage) (json.RawMessage, error) {
	var root map[string]json.RawMessage
	if len(settings) > 0 {
		if err := json.Unmarshal(settings, &root); err != nil {
			return nil, err
		}
	}
	if root == nil {
		root = map[string]json.RawMessage{}
	}
	if len(agentConfig) > 0 {
		root["agent_config"] = agentConfig
	}
	out, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func extractSettingsAgentConfig(settings json.RawMessage) (json.RawMessage, error) {
	return extractSettingsJSONObject(settings, "agent_config")
}

func agentConfigPatchFromRequest(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var payload map[string]string
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("invalid agent_config")
	}
	clean := map[string]string{}
	for key, value := range payload {
		if strings.TrimSpace(value) == "" {
			continue
		}
		clean[key] = strings.TrimSpace(value)
	}
	cfg, err := agentconfig.FromStringMap(clean)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(cfg.ToMap())
	if err != nil {
		return nil, err
	}
	return out, nil
}

func agentConfigMapFromPatch(raw json.RawMessage) (map[string]string, error) {
	patch, err := agentConfigPatchFromRequest(raw)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	if len(patch) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(patch, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Server) handleResolvedAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx := r.Context()
	defaults, err := s.defaultAgentConfig(ctx)
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
		dbCard, err := s.repo.GetScrumCard(ctx, projectID, cardID)
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
		resolved, err := s.resolvedAgentsForProjectCard(ctx, projectID, card)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"env_defaults": defaults.ToMap(),
			"fields":       defaults.FieldList(map[string]string{}),
			"resolved":     resolved,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"env_defaults": defaults.ToMap(),
		"fields":       defaults.FieldList(map[string]string{}),
	})
}

func (s *Server) handleAgentSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ctx := r.Context()
		cfg, err := s.defaultAgentConfig(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		stored := map[string]string{}
		if s.repo != nil {
			stored, err = s.repo.GetWorkspaceAgentConfig(ctx)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		path, err := resolveEnvFilePath()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"storage":   "database",
			"env_file":  path,
			"workspace": stored,
			"fields":    cfg.FieldList(map[string]string{}),
			"resolved":  cfg.ToMap(),
		})
	case http.MethodPut:
		if s.repo == nil {
			writeError(w, http.StatusServiceUnavailable, "database unavailable")
			return
		}
		var req struct {
			Values map[string]string `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		raw, err := json.Marshal(req.Values)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		clean, err := agentConfigMapFromPatch(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		stored, err := s.repo.SetWorkspaceAgentConfig(r.Context(), clean)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg, err := s.defaultAgentConfig(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		path, err := resolveEnvFilePath()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"storage":   "database",
			"env_file":  path,
			"workspace": stored,
			"fields":    cfg.FieldList(map[string]string{}),
			"resolved":  cfg.ToMap(),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) resolvedAgentsForProjectCard(ctx context.Context, projectID int64, card ScrumCard) (map[string]any, error) {
	if s.repo == nil || projectID <= 0 {
		resolved, source, err := s.resolveAgentConfig(ctx, model.Project{}, card)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"resolved": resolved.ToMap(),
			"source":   source,
			"fields":   resolved.FieldList(map[string]string{}),
			"system":   resolved.System(),
			"external": resolved.IsExternal(),
		}, nil
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	resolved, source, err := s.resolveAgentConfig(ctx, project, card)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"resolved": resolved.ToMap(),
		"source":   source,
		"fields":   resolved.FieldList(map[string]string{}),
		"system":   resolved.System(),
		"external": resolved.IsExternal(),
	}, nil
}
