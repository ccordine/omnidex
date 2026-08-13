package api

import (
	"embed"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

//go:embed web/panels/*.html
var uiPanelFS embed.FS

var (
	localizedUIPanelsOnce sync.Once
	localizedUIPanels     map[string]map[uiLocale]string
	localizedUIPanelsErr  error
)

var uiPanelNames = []string{"chat", "data", "projects", "jobs", "memory", "metrics", "admin"}

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
	if err := prepareLocalizedUIPanels(); err != nil {
		return "", err
	}
	locales, exists := localizedUIPanels[panel]
	if !exists {
		return "", fmt.Errorf("UI panel %q is not configured", panel)
	}
	html, exists := locales[locale]
	if !exists {
		return "", fmt.Errorf("UI panel %q locale %q is not configured", panel, locale)
	}
	return html, nil
}

func prepareLocalizedUIPanels() error {
	localizedUIPanelsOnce.Do(func() {
		localizedUIPanels = make(map[string]map[uiLocale]string, len(uiPanelNames))
		for _, panel := range uiPanelNames {
			path := "web/panels/" + panel + ".html"
			raw, err := uiPanelFS.ReadFile(path)
			if err != nil {
				localizedUIPanelsErr = fmt.Errorf("read UI panel %q: %w", panel, err)
				return
			}
			template := strings.TrimSpace(string(raw))
			template = strings.Replace(template, `class="hidden h-full min-h-0 flex-col"`, `class="flex h-full min-h-0 flex-col"`, 1)
			localizedUIPanels[panel] = make(map[uiLocale]string, len(supportedUILocaleOptions))
			for _, option := range supportedUILocaleOptions {
				rendered, err := renderLocalizedHTML(template, option.Code)
				if err != nil {
					localizedUIPanelsErr = fmt.Errorf("render UI panel %q locale %q: %w", panel, option.Code, err)
					return
				}
				localizedUIPanels[panel][option.Code] = rendered
			}
		}
	})
	return localizedUIPanelsErr
}
