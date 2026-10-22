package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/exactjson"
)

type codingAgentSettingsResponse struct {
	Configuration codingagent.Configuration `json:"configuration"`
	Models        []codingAgentModel        `json:"models"`
	Installation  codex.Installation        `json:"installation"`
	Source        string                    `json:"source"`
}

type codingAgentModel struct {
	Provider string `json:"provider"`
	codex.Model
}

func (server *Server) codingAgentClient() (*codex.Client, error) {
	return codex.New(server.providerConfig.CodexCommand, server.providerConfig.CodexDirectory)
}

func (server *Server) codingAgentSettings(ctx context.Context, client *codex.Client) (codingAgentSettingsResponse, error) {
	configuration, source, err := server.repo.LoadCodingAgentConfiguration(ctx)
	if err != nil {
		return codingAgentSettingsResponse{}, err
	}
	configuration.Mode = codingagent.Agents
	models, err := client.Models(ctx)
	if err != nil {
		return codingAgentSettingsResponse{}, err
	}
	installation, err := client.Installation(ctx)
	if err != nil {
		return codingAgentSettingsResponse{}, err
	}
	catalog, err := server.codingAgentModelCatalog(ctx, models)
	if err != nil {
		return codingAgentSettingsResponse{}, err
	}
	return codingAgentSettingsResponse{Configuration: configuration, Models: catalog, Installation: installation, Source: source}, nil
}

func (server *Server) handleCodingAgentSettings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		writeError(w, 405, "method not allowed")
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	server.codingAgentsMu.Lock()
	defer server.codingAgentsMu.Unlock()
	client, err := server.codingAgentClient()
	if err != nil {
		writeError(w, 503, err.Error())
		return
	}
	if r.Method == http.MethodPut {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, 400, "settings exceed their input boundary")
			return
		}
		var request struct {
			Configuration codingagent.Configuration `json:"configuration"`
		}
		if err := exactjson.ValidateObject(raw, &request, "agent settings"); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if err := json.Unmarshal(raw, &request); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if err := request.Configuration.Validate(); err != nil || request.Configuration.Mode != codingagent.Agents {
			writeError(w, 400, "settings require valid Agents routes")
			return
		}
		models, err := client.Models(r.Context())
		if err != nil {
			writeError(w, 503, err.Error())
			return
		}
		if err := validateCodingAgentModelOptions(request.Configuration, models); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		if request.Configuration.LedgerParser.Provider == "ollama" {
			if server.ollamaEndpoint() == "" {
				writeError(w, 400, "local parser requires a configured Ollama endpoint")
				return
			}
			installed, err := server.hasInstalledOllamaModel(r.Context(), request.Configuration.LedgerParser.Model)
			if err != nil {
				writeError(w, 503, err.Error())
				return
			}
			if !installed {
				writeError(w, 400, "parser model is absent from the installed Ollama catalog")
				return
			}
		}
		if err := server.repo.StoreCodingAgentConfiguration(r.Context(), request.Configuration); err != nil {
			writeError(w, 500, err.Error())
			return
		}
	}
	server.writeCodingAgentSettings(w, r, client)
}

func (server *Server) writeCodingAgentSettings(w http.ResponseWriter, r *http.Request, client *codex.Client) {
	settings, err := server.codingAgentSettings(r.Context(), client)
	if err != nil {
		writeError(w, 503, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, settings)
}

func validateCodingAgentModelOptions(configuration codingagent.Configuration, models []codex.Model) error {
	byID := map[string]codex.Model{}
	for _, model := range models {
		byID[model.ID] = model
	}
	routes := []codingagent.Route{configuration.Planner, configuration.Executor, configuration.Reviewer}
	parser := configuration.LedgerParser
	switch parser.Provider {
	case "codex":
		routes = append(routes, parser)
	case "ollama":
		if parser.Effort != "" || parser.ServiceTier != "" {
			return fmt.Errorf("local parser does not support Codex effort or service-tier options")
		}
	default:
		return fmt.Errorf("unsupported response parser provider %q", parser.Provider)
	}
	for _, route := range routes {
		model, known := byID[route.Model]
		if route.Provider != "codex" || !known {
			return fmt.Errorf("model %q is absent from the installed Codex catalog", route.Model)
		}
		effortKnown := route.Effort == ""
		for _, option := range model.Efforts {
			effortKnown = effortKnown || option.Value == route.Effort
		}
		if !effortKnown {
			return fmt.Errorf("model %q does not advertise effort %q", route.Model, route.Effort)
		}
		tierKnown := route.ServiceTier == "" || route.ServiceTier == "default"
		for _, tier := range model.ServiceTiers {
			tierKnown = tierKnown || tier.ID == route.ServiceTier
		}
		if !tierKnown {
			return fmt.Errorf("model %q does not advertise speed %q", route.Model, route.ServiceTier)
		}
	}
	return nil
}
