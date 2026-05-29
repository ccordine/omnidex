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
	path, err := resolveEnvFilePath()
	if err == nil {
		if values, err := readEnvFile(path); err == nil {
			cfg := agentconfig.Config{}
			for _, field := range agentconfig.Fields {
				if value := lookupEnvFileValue(values, field.EnvKeys); value != "" {
					cfg[field.Key] = value
				}
			}
			if len(cfg) > 0 {
				return cfg
			}
		}
	}
	return agentconfig.FromEnv()
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

func (s *Server) resolveAgentConfig(project model.Project, card ScrumCard) (agentconfig.Config, string) {
	env := s.envAgentConfig()
	projectCfg := s.projectAgentConfig(project)
	cardCfg := s.cardAgentConfig(card)
	resolved := agentconfig.Merge(env, projectCfg, cardCfg)
	source := "env"
	if len(projectCfg) > 0 {
		source = "project"
	}
	if len(cardCfg) > 0 {
		source = "card"
	}
	return resolved, source
}

func (s *Server) agentConfigJobMetadata(project model.Project, card ScrumCard) map[string]any {
	resolved, source := s.resolveAgentConfig(project, card)
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

func (s *Server) handleResolvedAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	env := s.envAgentConfig()
	projectID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("project_id")), 10, 64)
	cardID := strings.TrimSpace(r.URL.Query().Get("card_id"))
	card := ScrumCard{}
	if cardID != "" && s.repo != nil && projectID > 0 {
		if dbCard, err := s.repo.GetScrumCard(r.Context(), projectID, cardID); err == nil {
			card = dbScrumCardToAPI(dbCard)
		}
	}
	if projectID > 0 {
		resolved, err := s.resolvedAgentsForProjectCard(r.Context(), projectID, card)
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

func (s *Server) handleAgentSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg := s.envAgentConfig()
		path, _ := resolveEnvFilePath()
		writeJSON(w, http.StatusOK, map[string]any{
			"env_file": path,
			"fields":   cfg.FieldList(map[string]string{}),
			"resolved": cfg.ToMap(),
		})
	case http.MethodPut:
		var req struct {
			Values map[string]string `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
		path, err := resolveEnvFilePath()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		updates := map[string]string{}
		for _, field := range agentconfig.Fields {
			if value, ok := req.Values[field.Key]; ok && len(field.EnvKeys) > 0 {
				if field.Key == "agent_system" {
					value = agentconfig.Config{"agent_system": value}.System()
					if value == agentconfig.SystemOmnidex {
						value = "omnidex"
					}
				}
				updates[field.EnvKeys[0]] = strings.TrimSpace(value)
			}
		}
		if err := writeEnvFile(path, updates); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		cfg := s.envAgentConfig()
		writeJSON(w, http.StatusOK, map[string]any{
			"env_file": path,
			"fields":   cfg.FieldList(map[string]string{}),
			"resolved": cfg.ToMap(),
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) resolvedAgentsForProjectCard(ctx context.Context, projectID int64, card ScrumCard) (map[string]any, error) {
	if s.repo == nil || projectID <= 0 {
		resolved, source := s.resolveAgentConfig(model.Project{}, card)
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
	resolved, source := s.resolveAgentConfig(project, card)
	return map[string]any{
		"resolved": resolved.ToMap(),
		"source":   source,
		"fields":   resolved.FieldList(map[string]string{}),
		"system":   resolved.System(),
		"strict":   resolved.IsStrict(),
		"external": resolved.IsExternal(),
	}, nil
}
