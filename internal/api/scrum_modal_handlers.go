package api

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleScrumCardModal(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query, err := decodeScrumModalQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx, err := s.buildScrumModalContext(r, cardID, query)
	if err != nil {
		if errors.Is(err, queue.ErrScrumCardNotFound) {
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
		"files":                 append([]string{}, ctx.Files...),
		"dirs":                  append([]string{}, ctx.Dirs...),
		"file_path":             ctx.FilePath,
		"file_parent":           ctx.FileParent,
		"file_has_parent":       ctx.FileHasParent,
		"file_offset":           ctx.FileOffset,
		"file_has_previous":     ctx.FileHasPrevious,
		"file_previous_offset":  ctx.FilePreviousOffset,
		"file_has_more":         ctx.FileHasMore,
		"file_next_offset":      ctx.FileNextOffset,
		"play_queue":            ctx.PlayQueue,
		"pilot_pending":         ctx.PilotPending,
		"channel_before_cursor": ctx.ChannelBeforeCursor,
		"channel_has_more":      ctx.ChannelHasMore,
	}
}
