package api

import (
	"net/http"
)

func (s *Server) handleProjectPlanningChat(w http.ResponseWriter, _ *http.Request, _ int64) {
	writeRemovedInferenceAction(w, "project planning generation and history")
}

func (s *Server) handleProjectPlanningDrafts(w http.ResponseWriter, _ *http.Request, _ int64) {
	writeRemovedInferenceAction(w, "project planning draft promotion")
}
