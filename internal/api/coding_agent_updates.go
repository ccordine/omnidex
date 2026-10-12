package api

import "net/http"

func (server *Server) handleCodingAgentRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
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
	server.writeCodingAgentSettings(w, r, client)
}

func (server *Server) handleCodingAgentUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	server.codingAgentsMu.Lock()
	defer server.codingAgentsMu.Unlock()
	busy, err := server.repo.CodingAgentsBusy(r.Context(), server.providerConfig.LLMProvider == "codex")
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if busy {
		writeError(w, 409, "Codex is in use by an active job.")
		return
	}
	client, err := server.codingAgentClient()
	if err != nil {
		writeError(w, 503, err.Error())
		return
	}
	// Installation work belongs to the service, surviving browser navigation.
	if _, err := client.Update(server.lifecycleContext); err != nil {
		writeError(w, 503, err.Error())
		return
	}
	// Resolve again: the managed update may replace the native executable.
	client, err = server.codingAgentClient()
	if err != nil {
		writeError(w, 503, err.Error())
		return
	}
	server.writeCodingAgentSettings(w, r, client)
}
