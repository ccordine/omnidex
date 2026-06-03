package api

import (
	"net/http"
	"strings"
)

func (s *Server) handleScrumCardModal(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, err := s.buildScrumModalContext(r, cardID, r.URL.Query().Get("tab"))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "not found") {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	partial := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("partial")), "true") ||
		strings.TrimSpace(r.URL.Query().Get("partial")) == "1"
	bundle := renderScrumCardModalBundle(*ctx)
	if partial {
		bundle = renderScrumCardModalPartialBundle(*ctx)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"card": ctx.Card,
		"tab":  ctx.Tab,
		"html": map[string]any{
			"bundle": bundle,
		},
	})
}
