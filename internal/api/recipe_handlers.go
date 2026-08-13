package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/omni"
)

func (s *Server) handleRecipes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, err := exactChannelQueryInteger(r, "limit", 50, 1, omni.MaxRecipePageSize)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := omni.LoadRecipePage(s.recipeRoot(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"recipes": page.Recipes, "root": s.recipeRoot(), "offset": page.Offset,
		"has_more": page.HasMore, "next_offset": dataSourceNextOffset(page.Offset, len(page.Recipes), page.HasMore),
	})
}

func (s *Server) handleRecipeByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/recipes/")
	id = strings.Trim(id, "/")
	if id == "" {
		writeError(w, http.StatusNotFound, "recipe not found")
		return
	}
	recipe, err := omni.LoadRecipeByID(s.recipeRoot(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recipe": recipe})
}

func mergeRecipeCatalog(catalog omni.Recipe, override json.RawMessage) omni.Recipe {
	if len(override) == 0 {
		return catalog
	}
	var custom omni.Recipe
	if err := json.Unmarshal(override, &custom); err != nil {
		return catalog
	}
	out := catalog
	if strings.TrimSpace(custom.ID) != "" {
		out.ID = custom.ID
	}
	if strings.TrimSpace(custom.Description) != "" {
		out.Description = custom.Description
	}
	if strings.TrimSpace(custom.Operation) != "" {
		out.Operation = custom.Operation
	}
	if len(custom.RequiredProjectStates) > 0 {
		out.RequiredProjectStates = custom.RequiredProjectStates
	}
	if len(custom.ForbiddenUserOperations) > 0 {
		out.ForbiddenUserOperations = custom.ForbiddenUserOperations
	}
	if len(custom.Objectives) > 0 {
		out.Objectives = custom.Objectives
	}
	if len(custom.AllowedCommands) > 0 {
		out.AllowedCommands = custom.AllowedCommands
	}
	if len(custom.EvidenceRequired) > 0 {
		out.EvidenceRequired = custom.EvidenceRequired
	}
	if len(custom.CompletionChecks) > 0 {
		out.CompletionChecks = custom.CompletionChecks
	}
	return out
}
