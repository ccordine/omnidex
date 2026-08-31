package api

import (
	"net/http"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) handleProjectByID(w http.ResponseWriter, r *http.Request) {
	id, action := splitProjectPath(r.URL.Path)
	if id <= 0 {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if action != "" && !isLiveProjectAction(action) {
		writeError(w, http.StatusNotFound, "unknown project action")
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "projects require database")
		return
	}
	if routeProjectAction(s, w, r, id, action) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		s.handleProjectGet(w, r, id)
	case http.MethodPatch:
		s.handleProjectPatch(w, r, id)
	case http.MethodDelete:
		expectedUpdatedAt, err := decodeProjectDeleteRequest(w, r)
		if err != nil {
			writeError(w, projectRequestErrorStatus(err), err.Error())
			return
		}
		if err := s.repo.DeleteProjectAtRevision(r.Context(), id, expectedUpdatedAt); err != nil {
			writeProjectError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, projectDeleteResponse{
			CommitState: projectMutationCommitted, ProjectID: id,
			ExpectedUpdatedAt: expectedUpdatedAt.UTC().Format(time.RFC3339Nano), Deleted: true,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func isLiveProjectAction(action string) bool {
	switch action {
	case "play", "pause", "map", "map/scan", "git":
		return true
	default:
		return false
	}
}

func routeProjectAction(s *Server, w http.ResponseWriter, r *http.Request, id int64, action string) bool {
	switch action {
	case "":
		return false
	case "play":
		s.handleProjectPlay(w, r, id)
	case "pause":
		s.handleProjectPause(w, r, id)
	case "map", "map/scan":
		s.handleProjectMap(w, r, id, action)
	case "git":
		s.handleProjectGit(w, r, id)
	default:
		writeError(w, http.StatusNotFound, "unknown project action")
	}
	return true
}

func (s *Server) handleProjectGet(w http.ResponseWriter, r *http.Request, id int64) {
	project, err := s.repo.GetProject(r.Context(), id)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	summary, err := s.projectSummary(r.Context(), project)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resolved, err := s.resolvedModelsForProject(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": summary, "model_config": resolved})
}

func (s *Server) handleProjectPatch(w http.ResponseWriter, r *http.Request, id int64) {
	request, err := decodeProjectPatchRequest(w, r)
	if err != nil {
		writeError(w, projectRequestErrorStatus(err), err.Error())
		return
	}
	patch := model.ProjectPatch{}
	if request.Name.Present {
		value := request.Name.Value
		patch.Name = &value
	}
	if request.Location.Present {
		value, err := s.validateProjectLocation(r.Context(), request.Location.Value)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		patch.Location = &value
	}
	if request.Description.Present {
		value := request.Description.Value
		patch.Description = &value
	}
	if request.ModelConfig.Present {
		current, err := s.repo.GetProject(r.Context(), id)
		if err != nil {
			writeProjectError(w, err)
			return
		}
		modelConfig, err := modelConfigPatchFromRequest(request.ModelConfig.Value)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		settings, err := mergeProjectModelConfig(current.Settings, modelConfig)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		patch.Settings = &settings
	}
	project, err := s.repo.UpdateProjectAtRevision(r.Context(), id, request.ExpectedUpdatedAt, patch)
	if err != nil {
		writeProjectError(w, err)
		return
	}
	summary, err := s.projectSummary(r.Context(), project)
	if err != nil {
		writeCommittedProjectMutation(w, project, "summary", err)
		return
	}
	writeJSON(w, http.StatusOK, projectMutationResponse{
		CommitState: projectMutationCommitted,
		Project:     summary,
	})
}
