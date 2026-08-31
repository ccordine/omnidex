package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"
)

func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "projects require database")
		return
	}
	switch r.Method {
	case http.MethodGet:
		query, err := decodeProjectCollectionQuery(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		projects, err := s.repo.ListProjects(r.Context(), query.Limit+1, query.Offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		hasMore := len(projects) > query.Limit
		if hasMore {
			projects = projects[:query.Limit]
		}
		items := make([]map[string]any, 0, len(projects))
		for _, project := range projects {
			summary, err := s.projectSummary(r.Context(), project)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			items = append(items, summary)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"projects": items, "limit": query.Limit, "offset": query.Offset, "has_more": hasMore,
		})
	case http.MethodPost:
		if err := validateExactQuery(r); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.handleProjectCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProjectCreate(w http.ResponseWriter, r *http.Request) {
	request, err := decodeProjectCreateRequest(w, r)
	if err != nil {
		writeError(w, projectRequestErrorStatus(err), err.Error())
		return
	}
	location, err := s.validateProjectLocation(r.Context(), request.Location)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	project, err := s.repo.CreateProject(r.Context(), request.Name, location, request.Description)
	if err != nil {
		var postgresError *pgconn.PgError
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			writeError(w, http.StatusConflict, "project location already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	summary, err := s.projectSummary(r.Context(), project)
	if err != nil {
		writeCommittedProjectMutation(w, project, "summary", err)
		return
	}
	writeJSON(w, http.StatusCreated, projectMutationResponse{
		CommitState: projectMutationCommitted,
		Project:     summary,
	})
}
