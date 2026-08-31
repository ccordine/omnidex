package api

import (
	"embed"
	"fmt"
	"net/http"
	"strings"
)

//go:embed web/panels/*.html
var uiPanelFS embed.FS

var uiPanelNames = []string{"chat", "roleplay", "data", "projects", "jobs", "memory", "admin"}

type uiPanelResponse struct {
	Panel  string            `json:"panel"`
	Locale uiLocale          `json:"locale"`
	HTML   chatComponentHTML `json:"html"`
}

func (s *Server) handleUIPanel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err := validateUIStateQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	panel := normalizeUIPanel(r.URL.Query().Get("panel"))
	if panel == "" {
		writeError(w, http.StatusBadRequest, "invalid panel")
		return
	}
	sessionID, err := s.ensureUISessionCookie(w, r)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	state, _, err := s.loadUIState(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	state["panel"] = panel
	if err := mergeUIQueryState(state, r); err != nil {
		logUILocaleRejection(sessionID, "panel", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	locale, err := ensureUIStateLocale(state, r)
	if err != nil {
		logUILocaleRejection(sessionID, "panel", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.persistUIState(r.Context(), sessionID, state); err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	html, err := loadUIPanelHTML(panel, locale)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "panel localization failed: "+err.Error())
		return
	}
	setUILocaleResponseHeaders(w, locale)
	writeJSON(w, http.StatusOK, uiPanelResponse{
		Panel: panel, Locale: locale,
		HTML: chatComponentHTML{Bundle: renderRecyclrTemplateHTML("app-panel", html, "innerHTML")},
	})
}

func normalizeUIPanel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "chat"
	}
	for _, panel := range uiPanelNames {
		if value == panel {
			return panel
		}
	}
	return ""
}

func loadUIPanelHTML(panel string, locale uiLocale) (string, error) {
	if normalizeUIPanel(panel) != panel {
		return "", fmt.Errorf("UI panel %q is not configured", panel)
	}
	path := "web/panels/" + panel + ".html"
	raw, err := uiPanelFS.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read UI panel %q: %w", panel, err)
	}
	template := strings.TrimSpace(string(raw))
	template = strings.Replace(template, `class="hidden h-full min-h-0 flex-col"`, `class="flex h-full min-h-0 flex-col"`, 1)
	rendered, err := renderLocalizedHTML(template, locale)
	if err != nil {
		return "", fmt.Errorf("render UI panel %q locale %q: %w", panel, locale, err)
	}
	return rendered, nil
}
