package api

import (
	"embed"
	"net/http"
	"strings"
)

//go:embed web/panels/*.html
var uiPanelFS embed.FS

type uiPanelResponse struct {
	Panel string `json:"panel"`
	HTML  string `json:"html"`
}

func (s *Server) handleUIPanel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	panel := normalizeUIPanel(r.URL.Query().Get("panel"))
	if panel == "" {
		writeError(w, http.StatusBadRequest, "invalid panel")
		return
	}
	sessionID := s.ensureUISessionCookie(w, r)
	state, _, err := s.loadUIState(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	state["panel"] = panel
	mergeUIQueryState(state, r)
	if _, err := s.persistUIState(r.Context(), sessionID, state); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	html, err := loadUIPanelHTML(panel)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "panel template missing")
		return
	}
	writeJSON(w, http.StatusOK, uiPanelResponse{Panel: panel, HTML: html})
}

func normalizeUIPanel(value string) string {
	value = strings.TrimSpace(value)
	switch value {
	case "", "chat":
		return "chat"
	case "data", "projects", "jobs", "memory", "metrics", "admin":
		return value
	default:
		return ""
	}
}

func loadUIPanelHTML(panel string) (string, error) {
	path := "web/panels/" + panel + ".html"
	raw, err := uiPanelFS.ReadFile(path)
	if err != nil {
		return "", err
	}
	html := strings.TrimSpace(string(raw))
	html = strings.Replace(html, `class="hidden h-full min-h-0 flex-col"`, `class="flex h-full min-h-0 flex-col"`, 1)
	return html, nil
}
