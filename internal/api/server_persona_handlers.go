package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	coreURL, source, err := s.resolveCoreURL(r)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	dependencies := s.collectCoreDependencies(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{
		"status":          coreHealthStatus(dependencies),
		"time":            time.Now().UTC(),
		"queue_enabled":   s.repo != nil,
		"core_url":        coreURL,
		"core_url_source": source,
		"listen_addr":     strings.TrimSpace(s.listenAddr),
		"dependencies":    dependencies,
	})
}

func (s *Server) handleInstruct(w http.ResponseWriter, r *http.Request) {
	s.handlePersona(w, r, "instruct")
}

func (s *Server) handleRoleplay(w http.ResponseWriter, r *http.Request) {
	s.handlePersona(w, r, "roleplay")
}

func (s *Server) handleNarrate(w http.ResponseWriter, r *http.Request) {
	s.handlePersona(w, r, "narrate")
}

func (s *Server) handleReasoning(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, ok := decodePersonaRequest(w, r)
	if !ok {
		return
	}

	resolvedLLM, err := s.resolvePersonaLLM(req)
	if err != nil {
		var requestErr personaRequestError
		if errors.As(err, &requestErr) {
			writeError(w, requestErr.StatusCode, requestErr.Error())
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if strings.TrimSpace(resolvedLLM.Model) != "" {
		req.Model = strings.TrimSpace(resolvedLLM.Model)
	}

	started := time.Now()
	parseOutput, parseModel, err := s.runPersona(r.Context(), "reasoning_parse", req, nil, resolvedLLM.Client)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	deliberateOutput, deliberateModel, err := s.runPersona(r.Context(), "reasoning_deliberate", req, map[string]string{
		"Reasoning Parse Output": parseOutput,
	}, resolvedLLM.Client)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	finalOutput, finalModel, err := s.runPersona(r.Context(), "reasoning_final", req, map[string]string{
		"Reasoning Parse Output":      parseOutput,
		"Reasoning Deliberate Output": deliberateOutput,
	}, resolvedLLM.Client)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, personaResponse{
		Persona:   "reasoning",
		Model:     firstNonEmpty(finalModel, deliberateModel, parseModel, strings.TrimSpace(req.Model)),
		Output:    strings.TrimSpace(finalOutput),
		LatencyMS: time.Since(started).Milliseconds(),
		Stages: []personaStage{
			{Name: "parse", Output: parseOutput},
			{Name: "deliberate", Output: deliberateOutput},
			{Name: "final", Output: finalOutput},
		},
	})
}

func (s *Server) handlePersona(w http.ResponseWriter, r *http.Request, persona string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	req, ok := decodePersonaRequest(w, r)
	if !ok {
		return
	}

	resolvedLLM, err := s.resolvePersonaLLM(req)
	if err != nil {
		var requestErr personaRequestError
		if errors.As(err, &requestErr) {
			writeError(w, requestErr.StatusCode, requestErr.Error())
			return
		}
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if strings.TrimSpace(resolvedLLM.Model) != "" {
		req.Model = strings.TrimSpace(resolvedLLM.Model)
	}

	started := time.Now()
	if strings.EqualFold(persona, "instruct") && s.instructIntegration != nil {
		integrationResult, handled, statusCode, err := s.instructIntegration.Handle(r.Context(), req)
		if err != nil {
			writeError(w, statusCode, err.Error())
			return
		}
		if handled {
			writeJSON(w, statusCode, personaResponse{
				Persona:     "instruct",
				Model:       "integration:" + integrationResult.Action,
				Output:      integrationResult.Message,
				LatencyMS:   time.Since(started).Milliseconds(),
				Integration: &integrationResult,
			})
			return
		}
	}

	output, modelName, err := s.runPersona(r.Context(), persona, req, nil, resolvedLLM.Client)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, personaResponse{
		Persona:   persona,
		Model:     firstNonEmpty(modelName, strings.TrimSpace(req.Model)),
		Output:    strings.TrimSpace(output),
		LatencyMS: time.Since(started).Milliseconds(),
	})
}

func decodePersonaRequest(w http.ResponseWriter, r *http.Request) (personaRequest, bool) {
	var req personaRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return personaRequest{}, false
	}

	req.Model = strings.TrimSpace(req.Model)
	req.System = strings.TrimSpace(req.System)
	req.Prompt = strings.TrimSpace(req.Prompt)
	if req.LLM != nil {
		req.LLM.Provider = strings.TrimSpace(req.LLM.Provider)
		req.LLM.Model = strings.TrimSpace(req.LLM.Model)
		if req.LLM.Compatible != nil {
			req.LLM.Compatible.APIKey = strings.TrimSpace(req.LLM.Compatible.APIKey)
			req.LLM.Compatible.BaseURL = strings.TrimSpace(req.LLM.Compatible.BaseURL)
			req.LLM.Compatible.Organization = strings.TrimSpace(req.LLM.Compatible.Organization)
			req.LLM.Compatible.Project = strings.TrimSpace(req.LLM.Compatible.Project)
		}
	}
	if req.Prompt == "" && (req.Integration == nil || strings.TrimSpace(req.Integration.Instruction) == "") {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return personaRequest{}, false
	}
	return req, true
}
