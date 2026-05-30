package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/scrumcardllm"
)

type scrumCardTicketRequest struct {
	Prompt       string `json:"prompt"`
	CardPrompt   string `json:"card_prompt"`
	Ticket       string `json:"ticket"`
	Iterate      bool   `json:"iterate"`
	IterateNotes string `json:"iterate_notes"`
	Stream       bool   `json:"stream"`
}

func (s *Server) handleScrumCardTicket(w http.ResponseWriter, r *http.Request, cardID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req scrumCardTicketRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	card, board, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		writeError(w, http.StatusNotFound, "card not found")
		return
	}

	cardPrompt := strings.TrimSpace(req.CardPrompt)
	if cardPrompt == "" {
		cardPrompt = strings.TrimSpace(card.CardPrompt)
	}

	ticket := strings.TrimSpace(req.Ticket)
	shouldGenerate := ticket == "" || req.Iterate

	if shouldGenerate {
		if s.repo != nil && projectID > 0 && !req.Stream {
			ticketReq := scrumcardllm.TicketRequest{
				Prompt:       req.Prompt,
				CardPrompt:   cardPrompt,
				Ticket:       ticket,
				Iterate:      req.Iterate,
				IterateNotes: req.IterateNotes,
			}
			ticketModel := firstNonEmpty(s.ollamaDefaultModel, "llama3.2")
			job, updated, err := s.enqueueScrumCardLLMJob(r.Context(), projectID, card, scrumcardllm.ActionCardTicket, "", ticketModel, ticketReq)
			if err != nil {
				writeError(w, http.StatusConflict, err.Error())
				return
			}
			message := fmt.Sprintf("Queued card ticket job #%d", job.ID)
			if req.Iterate {
				message = fmt.Sprintf("Queued card ticket iteration job #%d", job.ID)
			}
			writeScrumCardLLMQueued(w, job, updated, message)
			return
		}
		system, user := scrumCardTicketPrompts(board, card, req)
		if req.Stream {
			s.handleScrumCardTicketStream(w, r, cardID, card, cardPrompt, system, user)
			return
		}
		generated, err := s.scrumLLMChat(r.Context(), llmContextSourceCardTicket, system, user, llmContextTelemetryMeta{CardID: cardID})
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		ticket = strings.TrimSpace(generated)
	}

	updated, err := s.persistScrumCardTicketDraft(r, cardID, cardPrompt, ticket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"card": updated, "ticket": ticket})
}

func (s *Server) handleScrumCardTicketStream(w http.ResponseWriter, r *http.Request, cardID string, card ScrumCard, cardPrompt, system, user string) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	emit := func(payload map[string]any) {
		blob, err := json.Marshal(payload)
		if err != nil {
			return
		}
		_, _ = w.Write(append(blob, '\n'))
		if flusher != nil {
			flusher.Flush()
		}
	}

	emit(map[string]any{"type": "start"})
	ticket, err := s.scrumLLMChatStream(r.Context(), llmContextSourceCardTicket, system, user, llmContextTelemetryMeta{CardID: cardID}, func(chunk string) error {
		emit(map[string]any{"type": "delta", "text": chunk})
		return nil
	})
	if err != nil {
		emit(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	ticket = strings.TrimSpace(ticket)
	updated, err := s.persistScrumCardTicketDraft(r, cardID, cardPrompt, ticket)
	if err != nil {
		emit(map[string]any{"type": "error", "message": err.Error()})
		return
	}
	emit(map[string]any{"type": "done", "card": updated, "ticket": ticket})
}

func scrumCardTicketPrompts(board ScrumBoard, card ScrumCard, req scrumCardTicketRequest) (string, string) {
	ticketReq := scrumcardllm.TicketRequest{
		Prompt:       req.Prompt,
		CardPrompt:   req.CardPrompt,
		Ticket:       req.Ticket,
		Iterate:      req.Iterate,
		IterateNotes: req.IterateNotes,
	}
	return scrumcardllm.CardTicketPrompts(scrumBoardContext(board), scrumCardContext(card), ticketReq)
}

func (s *Server) persistScrumCardTicketDraft(r *http.Request, cardID, cardPrompt, ticket string) (ScrumCard, error) {
	ticketRaw, _ := json.Marshal(ticket)
	promptRaw, _ := json.Marshal(cardPrompt)
	raw := map[string]json.RawMessage{
		"card_ticket": ticketRaw,
		"card_prompt": promptRaw,
	}
	patch := ScrumCard{CardTicket: ticket, CardPrompt: cardPrompt}
	return s.scrumUpdateCard(r, cardID, patch, raw)
}

func (s *Server) scrumLLMChat(ctx context.Context, source, system, user string, meta llmContextTelemetryMeta) (string, error) {
	modelName := firstNonEmpty(s.ollamaDefaultModel, "llama3.2")
	promptChars := llmPromptCharCount(system, user)
	if client := s.ollamaGenerationClient(); client != nil {
		generated, err := client.Chat(ctx, modelName, system, user)
		s.recordLLMContextUsage(ctx, source, modelName, "ollama", meta, promptChars, promptChars, false, 0, err)
		return generated, err
	}
	return s.scrumLLMGenerate(ctx, source, system, user, meta)
}

func (s *Server) scrumLLMChatStream(ctx context.Context, source, system, user string, meta llmContextTelemetryMeta, onChunk func(string) error) (string, error) {
	modelName := firstNonEmpty(s.ollamaDefaultModel, "llama3.2")
	promptChars := llmPromptCharCount(system, user)
	if client := s.ollamaGenerationClient(); client != nil {
		generated, err := client.ChatStream(ctx, modelName, system, user, onChunk)
		s.recordLLMContextUsage(ctx, source, modelName, "ollama", meta, promptChars, promptChars, false, 0, err)
		return generated, err
	}
	generated, err := s.scrumLLMGenerate(ctx, source, system, user, meta)
	if err != nil {
		return "", err
	}
	if onChunk != nil && generated != "" {
		if err := onChunk(generated); err != nil {
			return generated, err
		}
	}
	return generated, nil
}

func (s *Server) ollamaGenerationClient() *ollama.Client {
	if s.llmClient == nil {
		return nil
	}
	if client, ok := s.llmClient.(*ollama.Client); ok {
		return client
	}
	if routed, ok := s.llmClient.(*llm.RoutedClient); ok && routed.Generation != nil {
		if client, ok := routed.Generation.(*ollama.Client); ok {
			return client
		}
	}
	return nil
}
