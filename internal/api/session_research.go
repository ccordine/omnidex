package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func (s *Server) sessionResearch(w http.ResponseWriter, r *http.Request, channel model.ChannelID) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodGet {
		if err := validateExactQuery(r, "workspace_identity", "conversation_id", "offset"); err != nil {
			writeError(w, 400, err.Error())
			return
		}
		identity, err := requiredWorkspaceIdentityQuery(r)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		state, err := s.repo.ChannelSessionState(r.Context(), channel, identity)
		if err == nil {
			err = s.requireClientWorkspaceIdentity(r.Context(), state.WorkspaceRoot, identity)
		}
		if err != nil {
			writeError(w, userIntentStatus(err), err.Error())
			return
		}
		offset := 0
		if value := r.URL.Query().Get("offset"); value != "" {
			offset, err = strconv.Atoi(value)
			if err != nil || offset < 0 {
				writeError(w, 400, "research history offset must be nonnegative")
				return
			}
		}
		history, err := s.repo.ResearchConversationHistoryPage(r.Context(), channel, identity, model.LifecycleOperationID(r.URL.Query().Get("conversation_id")), offset)
		if err != nil {
			writeError(w, userIntentStatus(err), err.Error())
			return
		}
		writeJSON(w, 200, history)
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed")
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, 400, err.Error())
		return
	}
	var request sessionintent.ResearchRequest
	if err := decodeExactChannelJSON(w, r, "research question", 128<<10, &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	state, err := s.repo.ChannelSessionState(r.Context(), channel, request.WorkspaceIdentity)
	if err == nil {
		err = s.requireClientWorkspaceIdentity(r.Context(), state.WorkspaceRoot, request.WorkspaceIdentity)
	}
	if err != nil {
		writeError(w, userIntentStatus(err), err.Error())
		return
	}
	result, err := s.repo.Research(r.Context(), channel, request, func(ctx context.Context, current sessionintent.ResearchContext) (string, json.RawMessage, error) {
		configuration, _, err := s.repo.LoadCodingAgentConfiguration(ctx)
		if err != nil {
			return "", json.RawMessage(`[]`), err
		}
		provider, err := codex.New(s.providerConfig.CodexCommand, current.State.WorkspaceRoot, 2*time.Minute)
		if err != nil {
			return "", json.RawMessage(`[]`), err
		}
		return provider.Research(ctx, current.State.WorkspaceRoot, request.Text, current, configuration.Planner)
	})
	if err != nil {
		writeError(w, userIntentStatus(err), err.Error())
		return
	}
	writeJSON(w, 200, result)
}
