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

func (s *Server) envAgentConfig() agentconfig.Config {
	return s.defaultAgentConfig(context.Background())
}

func (s *Server) defaultAgentConfig(ctx context.Context) agentconfig.Config {
	cfg, _ := s.resolveAgentConfig(ctx, model.Project{}, ScrumCard{})
	return cfg
}

func (s *Server) workspaceAgentConfig(ctx context.Context) agentconfig.Config {
	if s.repo == nil {
		return agentconfig.Config{}
	}
	stored, err := s.repo.GetWorkspaceAgentConfig(ctx)
	if err != nil || len(stored) == 0 {
		return agentconfig.Config{}
	}
	return agentconfig.FromStringMap(stored)
}

func (s *Server) projectAgentConfig(project model.Project) agentconfig.Config {
	return agentconfig.FromSettingsJSON(project.Settings)
}

func (s *Server) cardAgentConfig(card ScrumCard) agentconfig.Config {
	if len(card.AgentConfig) == 0 {
		return agentconfig.Config{}
	}
	return agentconfig.FromJSON(card.AgentConfig)
}

// resolveAgentConfig merges: env → workspace (global DB) → project → card → instance.
func (s *Server) resolveAgentConfig(ctx context.Context, project model.Project, card ScrumCard, instance ...agentconfig.Config) (agentconfig.Config, string) {
	stack := agentconfig.Stack{
		Workspace:  s.workspaceAgentConfig(ctx),
		Project:    s.projectAgentConfig(project),
		Card:       s.cardAgentConfig(card),
		ProcessEnv: agentconfig.FromEnv(),
	}
	if path, err := resolveEnvFilePath(); err == nil {
		if values, err := readEnvFile(path); err == nil {
			stack.EnvFile = agentconfig.FromEnvFileValues(values)
		}
	}
	if len(instance) > 0 && len(instance[0]) > 0 {
		stack.Instance = instance[0]
	}
	return stack.Resolve()
}

func (s *Server) agentConfigJobMetadata(ctx context.Context, project model.Project, card ScrumCard, instance ...agentconfig.Config) map[string]any {
	resolved, source := s.resolveAgentConfig(ctx, project, card, instance...)
	payload := map[string]any{
		"agent_config":        resolved.ToMap(),
		"agent_config_source": source,
	}
	if resolved.IsExternal() {
		payload["external_agents_used"] = []string{resolved.ExternalAgentName()}
		payload["execution_agent"] = resolved.System()
	} else {
		payload["execution_agent"] = agentconfig.SystemOmnidex
	}
	if resolved.IsStrict() {
		payload["agent_strict"] = true
	}
	return payload
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

func extractSettingsAgentConfig(settings json.RawMessage) json.RawMessage {
	cfg := agentconfig.FromSettingsJSON(settings)
	if len(cfg) == 0 {
		return json.RawMessage(`{}`)
	}
	out, _ := json.Marshal(cfg.ToMap())
	return out
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
		if key == "agent_system" {
			value = agentconfig.Config{"agent_system": value}.System()
			if value == agentconfig.SystemOmnidex {
				continue
			}
		}
		if key == "agent_strict" && !(agentconfig.Config{"agent_strict": value}).IsStrict() {
			continue
		}
		clean[key] = strings.TrimSpace(value)
	}
	out, err := json.Marshal(clean)
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
	defaults := s.defaultAgentConfig(ctx)
	projectID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("project_id")), 10, 64)
	cardID := strings.TrimSpace(r.URL.Query().Get("card_id"))
	card := ScrumCard{}
	if cardID != "" && s.repo != nil && projectID > 0 {
		if dbCard, err := s.repo.GetScrumCard(ctx, projectID, cardID); err == nil {
			card = dbScrumCardToAPI(dbCard)
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
		cfg := s.defaultAgentConfig(ctx)
		stored := map[string]string{}
		if s.repo != nil {
			if values, err := s.repo.GetWorkspaceAgentConfig(ctx); err == nil {
				stored = values
			}
		}
		path, _ := resolveEnvFilePath()
		writeJSON(w, http.StatusOK, map[string]any{
			"storage":  "database",
			"env_file": path,
			"workspace": stored,
			"fields":   cfg.FieldList(map[string]string{}),
			"resolved": cfg.ToMap(),
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
		cfg := s.defaultAgentConfig(r.Context())
		path, _ := resolveEnvFilePath()
		writeJSON(w, http.StatusOK, map[string]any{
			"storage":  "database",
			"env_file": path,
			"workspace": stored,
			"fields":   cfg.FieldList(map[string]string{}),
			"resolved": cfg.ToMap(),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) resolvedAgentsForProjectCard(ctx context.Context, projectID int64, card ScrumCard) (map[string]any, error) {
	if s.repo == nil || projectID <= 0 {
		resolved, source := s.resolveAgentConfig(ctx, model.Project{}, card)
		return map[string]any{
			"resolved": resolved.ToMap(),
			"source":   source,
			"fields":   resolved.FieldList(map[string]string{}),
			"system":   resolved.System(),
			"strict":   resolved.IsStrict(),
			"external": resolved.IsExternal(),
		}, nil
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	resolved, source := s.resolveAgentConfig(ctx, project, card)
	return map[string]any{
		"resolved": resolved.ToMap(),
		"source":   source,
		"fields":   resolved.FieldList(map[string]string{}),
		"system":   resolved.System(),
		"strict":   resolved.IsStrict(),
		"external": resolved.IsExternal(),
	}, nil
}
