package api

import (
	"net/http"
	"strings"
)

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
	html, ok := extractUIPanelHTML(panel)
	if !ok {
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

func extractUIPanelHTML(panel string) (string, bool) {
	marker := `data-panel-name="` + panel + `"`
	markerIndex := strings.Index(uiSourceHTML, marker)
	if markerIndex < 0 {
		return "", false
	}
	start := strings.LastIndex(uiSourceHTML[:markerIndex], "<section")
	if start < 0 {
		return "", false
	}
	end := findSectionEnd(uiSourceHTML, start)
	if end < 0 {
		return "", false
	}
	html := uiSourceHTML[start:end]
	html = strings.Replace(html, `class="hidden h-full min-h-0 flex-col"`, `class="flex h-full min-h-0 flex-col"`, 1)
	html = strings.Replace(html, `class="hidden h-full min-h-0 flex-col"`, `class="flex h-full min-h-0 flex-col"`, 1)
	return strings.TrimSpace(html), true
}

func findSectionEnd(html string, start int) int {
	depth := 0
	cursor := start
	for cursor < len(html) {
		open := strings.Index(html[cursor:], "<section")
		close := strings.Index(html[cursor:], "</section>")
		if close < 0 {
			return -1
		}
		if open >= 0 && open < close {
			depth++
			cursor += open + len("<section")
			continue
		}
		depth--
		cursor += close + len("</section>")
		if depth == 0 {
			return cursor
		}
	}
	return -1
}
