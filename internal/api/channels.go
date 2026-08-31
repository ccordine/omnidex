package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

const (
	defaultChannelHistoryLimit = 24
)

type channelCreateRequest struct {
	ID                    string                     `json:"id"`
	Name                  string                     `json:"name"`
	Tags                  []string                   `json:"tags"`
	WorkspaceRoot         channelCreateWorkspaceRoot `json:"workspace_root,omitempty"`
	DataSourceID          channelCreateDataSourceID  `json:"data_source_id,omitempty"`
	Mode                  model.ChannelMode          `json:"mode"`
	RoleplayWorldName     channelCreateRoleplayName  `json:"roleplay_world_name,omitempty"`
	RoleplayViewpointName channelCreateRoleplayName  `json:"roleplay_viewpoint_name,omitempty"`
}

type channelCreateDataSourceID struct {
	Value model.DataSourceID
}

func (id *channelCreateDataSourceID) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return errors.New("channel data_source_id must be omitted or contain one canonical identity")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode channel data_source_id: %w", err)
	}
	candidate := model.DataSourceID(value)
	if err := candidate.Validate(); err != nil {
		return err
	}
	id.Value = candidate
	return nil
}

type channelCreateRoleplayName struct {
	Value   string
	Present bool
}

func (name *channelCreateRoleplayName) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return errors.New("roleplay creation names must be omitted or contain exact nonblank text")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode roleplay creation name: %w", err)
	}
	if len(value) == 0 || len(value) > model.MaxChannelNameBytes || !utf8.ValidString(value) ||
		strings.IndexByte(value, 0) >= 0 || value != strings.TrimSpace(value) {
		return fmt.Errorf("roleplay creation name must contain 1..%d exact nonblank UTF-8 bytes without NUL", model.MaxChannelNameBytes)
	}
	name.Value = value
	name.Present = true
	return nil
}

type channelMessageRequest struct {
	Prompt                   string                          `json:"prompt"`
	DelegatedDataAuthorityID channelDelegatedDataAuthorityID `json:"delegated_data_authority_id,omitempty"`
	RoleplayTurn             *roleplay.UserTurnRequest       `json:"roleplay_turn,omitempty"`
}

type channelDelegatedDataAuthorityID struct {
	Value   string
	Present bool
}

func (id *channelDelegatedDataAuthorityID) UnmarshalJSON(raw []byte) error {
	if string(raw) == "null" {
		return errors.New("delegated_data_authority_id must be omitted or contain one canonical identity")
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("decode delegated_data_authority_id: %w", err)
	}
	id.Value = value
	id.Present = true
	return nil
}

type channelMessageResponse struct {
	Channel     model.Channel        `json:"channel"`
	UserMessage model.ChannelMessage `json:"user_message"`
	Job         model.Job            `json:"job"`
}

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
	workspaceRoot, workspaceErr := s.resolveChannelCreateWorkspaceRoot(req.WorkspaceRoot)
	if workspaceErr != nil {
		writeError(w, http.StatusServiceUnavailable, workspaceErr.Error())
		return
	}
	channelInput := model.Channel{
		ID: model.ChannelID(req.ID), Scope: model.ChannelScopeUser, Name: req.Name, Tags: append([]string(nil), req.Tags...),
		WorkspaceRoot: workspaceRoot, DataSourceID: req.DataSourceID.Value, Mode: req.Mode,
	}
	var channel model.Channel
	var err error
	switch req.Mode {
	case model.ChannelModeAssistant:
		if req.RoleplayWorldName.Present || req.RoleplayViewpointName.Present {
			writeError(w, http.StatusBadRequest, "assistant channel creation cannot carry roleplay names")
			return
		}
		channel, err = s.repo.CreateChannel(r.Context(), channelInput)
	case model.ChannelModeRoleplay:
		if req.DataSourceID.Value != "" {
			writeError(w, http.StatusBadRequest, "roleplay channel cannot bind a real-world data source")
			return
		}
		if !req.RoleplayWorldName.Present || !req.RoleplayViewpointName.Present {
			writeError(w, http.StatusBadRequest, "roleplay channel creation requires exact world and viewpoint names")
			return
		}
		channel, err = s.repo.CreateRoleplayChannel(
			r.Context(), channelInput, req.RoleplayWorldName.Value, req.RoleplayViewpointName.Value,
		)
	default:
		writeError(w, http.StatusBadRequest, "channel mode must be exactly assistant or roleplay")
		return
	}
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
	channels, err := s.repo.ListChannels(r.Context(), model.ChannelScopeUser, limit, offset)
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
	if len(parts) > 1 && parts[1] == "session" {
		if len(parts) > 2 {
			writeError(w, http.StatusNotFound, "channel route not found")
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.getChannelSession(w, r, channelID)
		return
	}
	if len(parts) > 1 && parts[1] == "roleplay" {
		s.handleRoleplaySimulationChannel(w, r, channelID, parts[2:])
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
	channel, err := s.repo.GetChannel(r.Context(), channelID)
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
	var channel model.Channel
	var userMessage model.ChannelMessage
	var job model.Job
	if req.RoleplayTurn == nil {
		userMessage, job, err = s.repo.EnqueueChannelTurnWithDataAuthority(
			r.Context(), channelID, prompt, req.DelegatedDataAuthorityID.Value,
		)
		if err == nil {
			channel, err = s.repo.GetChannel(r.Context(), channelID)
		}
	} else {
		channel, err = s.repo.GetChannel(r.Context(), channelID)
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
		switch channel.Mode {
		case model.ChannelModeAssistant:
			writeError(w, http.StatusBadRequest, "assistant channel turn cannot carry roleplay user authority")
			return
		case model.ChannelModeRoleplay:
			if req.DelegatedDataAuthorityID.Present {
				writeError(w, http.StatusBadRequest, "roleplay channel turn cannot carry delegated data authority")
				return
			}
			if err = req.RoleplayTurn.ValidateForExactText(prompt); err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			userMessage, job, err = s.repo.EnqueueRoleplayChannelTurn(
				r.Context(), channel.ID, prompt, *req.RoleplayTurn,
			)
		default:
			writeError(w, http.StatusInternalServerError, "channel has unsupported stored mode")
			return
		}
	}
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, queue.ErrChannelTurnActive),
			errors.Is(err, roleplay.ErrSimulationNotConfigured),
			errors.Is(err, roleplay.ErrSimulationStaleRevision),
			errors.Is(err, roleplay.ErrSimulationConflict):
			status = http.StatusConflict
		case errors.Is(err, roleplay.ErrSimulationUnknown),
			errors.Is(err, roleplay.ErrSimulationAmbiguous),
			errors.Is(err, roleplay.ErrSimulationIllegal):
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	log.Printf(
		"channel turn accepted channel=%s mode=%s message=%d job=%d",
		channel.ID, channel.Mode, userMessage.ID, job.ID,
	)
	s.publishChannelJobProgress(channel.ID, job.ID, realtimeJobQueued, "Job queued")
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
	return exactIntegerValue(values[0], key, minimum, maximum)
}

func exactIntegerValue(raw, label string, minimum, maximum int) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil || strconv.Itoa(value) != raw || value < minimum || value > maximum {
		return 0, errors.New(label + " is outside its accepted integer range")
	}
	return value, nil
}
