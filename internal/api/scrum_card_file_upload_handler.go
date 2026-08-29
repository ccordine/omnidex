package api

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleScrumFiles(w http.ResponseWriter, _ *http.Request) {
	writeRemovedInferenceAction(w, "unpaginated Scrum file inventory")
}

func (s *Server) handleScrumCardFiles(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method == http.MethodGet {
		page, err := s.loadScrumCardFilePage(r, cardID)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, queue.ErrScrumCardNotFound) {
				status = http.StatusNotFound
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, page)
		return
	}
	if r.Method == http.MethodPost {
		writeError(w, http.StatusGone, "Scrum file upload is retired until workspace publication has a durable mutation journal")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}
