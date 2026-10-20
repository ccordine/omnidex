package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gryph/omnidex/internal/codex"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/sessionintent"
	"github.com/gryph/omnidex/internal/userintent"
	"github.com/jackc/pgx/v5"
)

type userIntentExecutionRequest struct {
	OperationID       model.LifecycleOperationID `json:"operation_id"`
	WorkspaceIdentity string                     `json:"workspace_identity"`
}

func (s *Server) postUserIntent(w http.ResponseWriter, r *http.Request, channelID model.ChannelID) {
	var request sessionintent.Request
	if err := decodeExactChannelJSON(w, r, "user intention", 128<<10, &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if request.State.ChannelID != channelID {
		writeError(w, 400, "user intention belongs to a different channel")
		return
	}
	if err := s.requireClientWorkspaceIdentity(r.Context(), request.State.WorkspaceRoot, request.State.WorkspaceIdentity); err != nil {
		writeError(w, 409, err.Error())
		return
	}
	result, err := s.repo.InterpretUserIntent(r.Context(), request, func(ctx context.Context, current sessionintent.Context) (sessionintent.Result, json.RawMessage, error) {
		request := request
		if request.Mode == sessionintent.Interact && request.Recipient == "" {
			request.Recipient = sessionintent.InteractionRecipient(current)
		}
		if err := request.ValidateRouting(current); err != nil {
			return sessionintent.Result{}, nil, err
		}
		if request.Mode == sessionintent.Interact && request.Recipient != sessionintent.Approval && (request.Recipient != sessionintent.Native || len(current.NativeChoices) == 0) {
			result, err := sessionintent.DirectContent(request, current)
			return result, json.RawMessage(`[]`), err
		}
		ask, evidence, err := s.userIntentModel(ctx, current)
		if err != nil {
			return sessionintent.Result{}, nil, err
		}
		result, err := interpretSessionUserIntent(ctx, request, current, ask)
		raw, encodeErr := json.Marshal(*evidence)
		if err == nil {
			err = encodeErr
		}
		return result, raw, err
	})
	if err != nil {
		log.Printf("user intention failed channel=%s operation=%s error=%v", channelID, request.OperationID, err)
		writeError(w, userIntentStatus(err), err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) executeUserIntent(w http.ResponseWriter, r *http.Request, channelID model.ChannelID) {
	var request userIntentExecutionRequest
	if err := decodeExactChannelJSON(w, r, "user action execution", 4096, &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	state, err := s.repo.ChannelSessionState(r.Context(), channelID, request.WorkspaceIdentity)
	if err == nil {
		err = s.requireClientWorkspaceIdentity(r.Context(), state.WorkspaceRoot, request.WorkspaceIdentity)
	}
	if err != nil {
		writeError(w, userIntentStatus(err), err.Error())
		return
	}
	receipt, err := s.repo.ExecuteUserIntent(r.Context(), request.OperationID, channelID, request.WorkspaceIdentity)
	if err != nil {
		writeError(w, userIntentStatus(err), err.Error())
		return
	}
	if receipt.JobID > 0 {
		s.publishChannelJobProgress(channelID, receipt.JobID, realtimeJobChanged, receipt.Summary)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, receipt)
}

func userIntentStatus(err error) int {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return 404
	case errors.Is(err, queue.ErrUserIntentStale), errors.Is(err, queue.ErrLifecycleOperationConflict), errors.Is(err, queue.ErrChannelSessionWorkspace), errors.Is(err, queue.ErrCodingLedgerRevision), errors.Is(err, queue.ErrCodingLedgerState):
		return 409
	default:
		return 422
	}
}

type userIntentModelEvidence struct {
	ElapsedMS  int64                  `json:"elapsed_ms"`
	InputBytes int                    `json:"input_bytes"`
	Prompt     string                 `json:"prompt"`
	Model      string                 `json:"model"`
	Generation llm.PreparedGeneration `json:"generation"`
}

func (s *Server) userIntentModel(ctx context.Context, current sessionintent.Context) (userintent.Ask, *[]userIntentModelEvidence, error) {
	configuration, _, err := s.repo.LoadCodingAgentConfiguration(ctx)
	if err != nil {
		return nil, nil, err
	}
	if current.JobID > 0 {
		details, err := s.repo.CurrentJobDetails(ctx, current.JobID)
		if err != nil {
			return nil, nil, err
		}
		configuration, err = codingagent.ConfigurationFromMetadata(details.Job.Metadata)
		if err != nil {
			return nil, nil, err
		}
	}
	route := configuration.LedgerParser
	var provider llm.ExactStationClient
	switch route.Provider {
	case "codex":
		client, err := s.userIntentProvider()
		if err != nil {
			return nil, nil, err
		}
		provider = client
	case "ollama":
		if s.ollamaEndpoint() == "" {
			return nil, nil, fmt.Errorf("user interpreter has no configured Ollama endpoint")
		}
		provider = ollama.New(s.ollamaEndpoint(), route.Model, "", 45*time.Second)
	default:
		return nil, nil, fmt.Errorf("user interpreter provider %q is unavailable", route.Provider)
	}
	evidence := []userIntentModelEvidence{}
	ask := func(ctx context.Context, prompt string) (string, error) {
		if len(prompt) > 128<<10 {
			return "", fmt.Errorf("user semantic question exceeds 128 KiB")
		}
		prepared, err := llm.PrepareNativeRequest(provider, llm.PreparedModel{Protocol: llm.ExactPreparedProtocolPlainCompletionV4, BaseModel: route.Model, ContextModel: route.Model, Prompt: prompt, MaxOutputTokens: 2048, OutputLimitMode: llm.ExactPreparedOutputLimitExplicit, ContextTokens: 32768})
		if err != nil {
			return "", err
		}
		started := time.Now()
		result, err := provider.GeneratePreparedExact(ctx, prepared)
		evidence = append(evidence, userIntentModelEvidence{Prompt: prompt, Model: route.Model, Generation: result, ElapsedMS: time.Since(started).Milliseconds(), InputBytes: len(prompt)})
		if err != nil {
			return "", err
		}
		if err := llm.ValidateExactPreparedGenerationForRequest(prepared, result); err != nil {
			return "", err
		}
		return result.Content, nil
	}
	return ask, &evidence, nil
}

func (s *Server) userIntentProvider() (*codex.Client, error) {
	s.userParserMu.Lock()
	defer s.userParserMu.Unlock()
	if s.userParser != nil {
		return s.userParser, nil
	}
	client, err := codex.NewParser(s.lifecycleContext, s.providerConfig.CodexCommand, s.providerConfig.CodexDirectory, 45*time.Second)
	if err != nil {
		return nil, err
	}
	s.userParser = client
	return client, nil
}

// CloseUserIntentProvider joins parser shutdown after lifecycle cancellation.
func (s *Server) CloseUserIntentProvider() {
	s.userParserMu.Lock()
	client := s.userParser
	s.userParserMu.Unlock()
	if client != nil {
		_ = client.Close()
	}
}
