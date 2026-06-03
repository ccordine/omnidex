package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/scrumcardllm"
)

type scrumCardTicketRequest struct {
	Prompt       string `json:"prompt"`
	CardPrompt   string `json:"card_prompt"`
	Ticket       string `json:"ticket"`
	Iterate      bool   `json:"iterate"`
	IterateNotes string `json:"iterate_notes"`
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
	card, _, projectID, err := s.scrumGetCard(r, cardID)
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
		if s.repo == nil || projectID <= 0 {
			writeError(w, http.StatusServiceUnavailable, "card ticket generation requires a project database")
			return
		}
		ticketReq := scrumcardllm.TicketRequest{
			Prompt:       req.Prompt,
			CardPrompt:   cardPrompt,
			Ticket:       ticket,
			Iterate:      req.Iterate,
			IterateNotes: req.IterateNotes,
		}
		ticketReq.PlanningMode = true
		ticketModel := s.scrumCardTicketModel(r.Context(), projectID)
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

	updated, err := s.persistScrumCardTicketDraft(r, cardID, cardPrompt, ticket)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"card":   updated,
		"ticket": ticket,
		"html": map[string]any{
			"bundle": renderScrumCardLLMSectionBundle(updated),
		},
	})
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
