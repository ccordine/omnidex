package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

const (
	defaultChannelHistoryLimit = 24
)

type channelStore interface {
	CreateChannel(ctx context.Context, channel model.Channel) (model.Channel, error)
	GetChannel(ctx context.Context, id model.ChannelID) (model.Channel, error)
	ListChannels(ctx context.Context, scope model.ChannelScope, limit, offset int) ([]model.Channel, error)
	ListChannelMessages(ctx context.Context, channelID model.ChannelID, limit int, beforeID *int64) (model.ChannelMessagePage, error)
}

type channelCreateRequest struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Tags          []string `json:"tags"`
	WorkspaceRoot string   `json:"workspace_root"`
}

type channelMessageRequest struct {
	Prompt string `json:"prompt"`
}

type channelMessageResponse struct {
	Channel     model.Channel        `json:"channel"`
	UserMessage model.ChannelMessage `json:"user_message"`
	Job         model.Job            `json:"job"`
}

type enqueueChannelTurnFunc func(
	context.Context,
	model.ChannelID,
	string,
) (model.ChannelMessage, model.Job, error)

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.createChannel(w, r)
	case http.MethodGet:
		s.listChannels(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) createChannel(w http.ResponseWriter, r *http.Request) {
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req channelCreateRequest
	if err := decodeExactChannelJSON(w, r, "channel create request", maxChannelCreateBodyBytes, &req); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	channel, err := s.channelStore.CreateChannel(r.Context(), model.Channel{
		ID: model.ChannelID(req.ID), Scope: model.ChannelScopeUser, Name: req.Name, Tags: append([]string(nil), req.Tags...),
		WorkspaceRoot: req.WorkspaceRoot,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, queue.ErrChannelAlreadyExists) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"channel": channel})
}

func (s *Server) listChannels(w http.ResponseWriter, r *http.Request) {
	if err := validateExactQuery(r, "scope", "limit", "offset"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.URL.Query().Get("scope") != string(model.ChannelScopeUser) {
		writeError(w, http.StatusBadRequest, "channel scope must be exactly user")
		return
	}
	limit, err := exactChannelQueryInteger(r, "limit", 50, 1, 200)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1<<30)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channels, err := s.channelStore.ListChannels(r.Context(), model.ChannelScopeUser, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": channels})
}

func (s *Server) handleChannelByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/v1/channels/")
	rest = strings.Trim(rest, "/")
	if rest == "" {
		writeError(w, http.StatusBadRequest, "channel id is required")
		return
	}
	parts := strings.Split(rest, "/")
	channelID := model.ChannelID(parts[0])
	if err := channelID.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(parts) > 1 && parts[1] == "messages" {
		if len(parts) > 2 {
			writeError(w, http.StatusNotFound, "channel route not found")
			return
		}
		switch r.Method {
		case http.MethodPost:
			s.postChannelMessage(w, r, channelID)
		case http.MethodGet:
			s.listChannelMessages(w, r, channelID)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) > 1 {
		writeError(w, http.StatusNotFound, "channel route not found")
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channel, err := s.channelStore.GetChannel(r.Context(), channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"channel": channel})
}

func (s *Server) postChannelMessage(w http.ResponseWriter, r *http.Request, channelID model.ChannelID) {
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req channelMessageRequest
	if err := decodeExactChannelJSON(w, r, "channel message request", maxChannelMessageBodyBytes, &req); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	prompt, err := requireFreeFormAuthority(req.Prompt, "prompt")
	if err != nil {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	channel, err := s.channelStore.GetChannel(r.Context(), channelID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if channel.Scope != model.ChannelScopeUser {
		writeError(w, http.StatusNotFound, "channel not found")
		return
	}
	if s.enqueueChannelTurn == nil {
		writeError(w, http.StatusServiceUnavailable, "channel job queue is unavailable")
		return
	}
	userMessage, job, err := s.enqueueChannelTurn(r.Context(), channel.ID, prompt)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, queue.ErrChannelTurnActive) {
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, channelMessageResponse{
		Channel: channel, UserMessage: userMessage, Job: job,
	})
}

func exactChannelQueryInteger(r *http.Request, key string, fallback, minimum, maximum int) (int, error) {
	values, exists := r.URL.Query()[key]
	if !exists {
		return fallback, nil
	}
	if len(values) != 1 || values[0] == "" {
		return 0, errors.New(key + " must be one canonical integer")
	}
	raw := values[0]
	value, err := strconv.Atoi(raw)
	if err != nil || strconv.Itoa(value) != raw || value < minimum || value > maximum {
		return 0, errors.New(key + " is outside its accepted integer range")
	}
	return value, nil
}
