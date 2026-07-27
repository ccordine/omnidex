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
	writeJSON(w, http.StatusOK, scrumModalPayload(ctx))
}

func scrumModalPayload(ctx *scrumModalRenderContext) map[string]any {
	return map[string]any{
		"card":                  ctx.Card,
		"board":                 ctx.Board,
		"tab":                   ctx.Tab,
		"project_id":            ctx.ProjectID,
		"files":                 ctx.Files,
		"dirs":                  ctx.Dirs,
		"play_queue":            ctx.PlayQueue,
		"model_fields":          ctx.ModelFields,
		"model_source":          ctx.ModelSource,
		"model_overrides":       ctx.ModelOverrides,
		"agent_fields":          ctx.AgentFields,
		"agent_source":          ctx.AgentSource,
		"agent_system":          ctx.AgentSystem,
		"agent_overrides":       ctx.AgentOverrides,
		"recipes":               ctx.Recipes,
		"project_recipe_id":     ctx.ProjectRecipeID,
		"project_recipe":        ctx.ProjectRecipe,
		"pilot_pending":         ctx.PilotPending,
		"channel_before_cursor": ctx.ChannelBeforeCursor,
		"channel_has_more":      ctx.ChannelHasMore,
	}
}
