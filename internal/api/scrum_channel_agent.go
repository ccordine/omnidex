package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
)

type scrumChannelDispatchResult struct {
	Card   ScrumCard
	Action string
	Agent  string
}

// buildScrumChannelAgentInstruction minifies channel history then builds the job instruction.
// Same card config (models, agent, fallbacks) is applied later via scrumPlayMetadata / enrichJobMetadata.
func buildScrumChannelAgentInstruction(board ScrumBoard, card ScrumCard, userMessage string, ctx scrumPilotPromptContext) string {
	prompt := buildScrumPilotChatPrompt(board, card, userMessage, ctx)
	return strings.TrimSpace("Channel message for this card (use the card's configured agent and models):\n\n" + prompt)
}

// scrumChannelResumeQuery picks the latest user steer note, or a resume default when Play
// is pressed after a long gap — the card channel is the persistent conversation for this task.
func scrumChannelResumeQuery(card ScrumCard) string {
	chat := sortScrumChatChronological(card.Chat)
	for i := len(chat) - 1; i >= 0; i-- {
		if normalizeScrumChannelRole(chat[i].Role) != "user" {
			continue
		}
		if text := strings.TrimSpace(chat[i].Content); text != "" {
			return text
		}
	}
	return "Resume this card task from prior channel work."
}

// buildScrumPlayInstructionWithHistory attaches minified card channel history so Play and
// channel chat share one persistent thread — come back weeks later and the agent sees where you left off.
func (s *Server) buildScrumPlayInstructionWithHistory(ctx context.Context, board ScrumBoard, card ScrumCard) string {
	base := buildScrumPlayInstruction(board, card)
	if len(card.Chat) == 0 {
		return base
	}
	query := scrumChannelResumeQuery(card)
	pilotContext := s.summarizeScrumPilotChannel(ctx, board, card, query, nil)
	channelBlock := buildScrumPilotChatPrompt(board, card, query, pilotContext)
	return strings.TrimSpace(base + "\n\nCard channel history (persistent thread for this card — resume where work left off):\n\n" + channelBlock)
}

func (s *Server) dispatchScrumChannelMessage(
	r *http.Request,
	board ScrumBoard,
	projectID int64,
	card ScrumCard,
	userMessage string,
) (scrumChannelDispatchResult, error) {
	pilotContext := s.summarizeScrumPilotChannel(r.Context(), board, card, userMessage, nil)
	instruction := buildScrumChannelAgentInstruction(board, card, userMessage, pilotContext)
	s.recordScrumPilotContextShrink(r.Context(), projectID, card, board, userMessage, pilotContext, instruction)

	agent := s.scrumCardResolvedAgent(r.Context(), projectID, card).System()
	out := scrumChannelDispatchResult{Card: card, Agent: agent}

	prepared, err := s.prepareScrumCardForChannelDispatch(r.Context(), projectID, card)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	card = prepared
	out.Card = card

	if card.PlayState == scrumPlayRunning && strings.TrimSpace(card.JobID) != "" && s.repo != nil {
		jobID, err := parseJobID(card.JobID)
		if err != nil {
			return scrumChannelDispatchResult{}, err
		}
		if _, err := s.repo.InterruptJob(r.Context(), jobID, instruction); err == nil {
			card = moveScrumCardToInProgress(card)
			card = appendScrumChannelEvent(card, "system", "Channel steer sent to running agent")
			saved, err := s.persistScrumCard(r, projectID, card)
			if err != nil {
				return scrumChannelDispatchResult{}, err
			}
			out.Card = saved
			out.Action = "steered"
			return out, nil
		}
		if _, err := s.repo.SubmitJobFeedback(r.Context(), jobID, instruction); err == nil {
			card = moveScrumCardToInProgress(card)
			card = appendScrumChannelEvent(card, "system", "Channel message sent to waiting agent")
			saved, err := s.persistScrumCard(r, projectID, card)
			if err != nil {
				return scrumChannelDispatchResult{}, err
			}
			out.Card = saved
			out.Action = "feedback"
			return out, nil
		}
		// Running job could not accept steer — cancel and start a fresh channel run.
		_, _ = s.repo.CancelJob(r.Context(), jobID, "channel message started new run")
		card.JobID = ""
		card.PlayState = ""
		card.QueueOrder = 0
	}

	card = moveScrumCardToInProgress(card)
	started, err := s.enqueueScrumCardAgentRun(r, board, projectID, card, agentconfig.Config{}, instruction, true)
	if err != nil {
		return scrumChannelDispatchResult{}, err
	}
	out.Card = started
	out.Action = "started"
	return out, nil
}

func (s *Server) enqueueScrumCardAgentRun(
	r *http.Request,
	board ScrumBoard,
	projectID int64,
	card ScrumCard,
	instance agentconfig.Config,
	instruction string,
	channelOrigin bool,
) (ScrumCard, error) {
	instruction = strings.TrimSpace(sanitizeScrumChannelText(instruction))
	if instruction == "" {
		return ScrumCard{}, fmt.Errorf("instruction is required")
	}

	metadata, pulled, metaErr := s.scrumPlayMetadata(r.Context(), board, card, projectID, instance)
	if metaErr != nil {
		return ScrumCard{}, metaErr
	}

	if s.repo != nil && projectID > 0 {
		project, err := s.repo.GetProject(r.Context(), projectID)
		if err != nil {
			return ScrumCard{}, err
		}
		if err := s.validateScrumPlayAgent(r.Context(), project, card, instance); err != nil {
			return ScrumCard{}, err
		}
	}

	if s.repo == nil {
		output, directErr := s.runScrumDirectInstruct(r.Context(), instruction, board, card)
		if directErr != nil {
			return ScrumCard{}, directErr
		}
		if channelOrigin {
			card = appendScrumChannelEvent(card, "system", "Agent run completed (direct mode)")
		}
		card = appendScrumChannelEvent(card, "assistant", output)
		card.Column = scrumChannelCompletionColumn(card.Column, channelOrigin)
		card.PlayState = ""
		card.QueueOrder = 0
		return s.persistScrumCard(r, projectID, card)
	}

	if channelOrigin {
		metadata = scrumChannelJobMetadata(metadata, card.Column)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	job, err := s.repo.EnqueueJob(ctx, instruction, "scrum", metadata)
	cancel()
	if err != nil {
		return ScrumCard{}, err
	}

	if channelOrigin {
		card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Job #%d queued from channel (card config)", job.ID))
	} else {
		card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Job #%d queued", job.ID))
		card = appendScrumChannelEvent(card, "user", instruction)
	}
	if len(pulled) > 0 {
		card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Models: %s", strings.Join(pulled, ", ")))
	}
	var meta map[string]any
	if err := json.Unmarshal(metadata, &meta); err == nil {
		if agent, _ := meta["execution_agent"].(string); strings.TrimSpace(agent) != "" {
			card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Execution agent: %s", strings.TrimSpace(agent)))
		}
		if source, _ := meta["agent_config_source"].(string); strings.TrimSpace(source) != "" {
			card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Agent config source: %s", strings.TrimSpace(source)))
		}
	}

	card.JobID = fmt.Sprintf("%d", job.ID)
	card.Column = scrumChannelPlayColumn(card.Column, channelOrigin)
	card.PlayState = scrumPlayRunning
	card.QueueOrder = 0
	return s.persistScrumCard(r, projectID, card)
}

// scrumChannelPlayColumn moves channel-origin runs to in_progress regardless of prior column.
func scrumChannelPlayColumn(current string, channelOrigin bool) string {
	if channelOrigin {
		return "in_progress"
	}
	return "in_progress"
}

func moveScrumCardToInProgress(card ScrumCard) ScrumCard {
	card.Column = "in_progress"
	card.QueueOrder = 0
	if card.PlayState == scrumPlayQueued || card.PlayState == scrumPlayPaused {
		card.PlayState = ""
	}
	return card
}

// scrumChannelCompletionColumn returns the column after a channel-origin run finishes.
func scrumChannelCompletionColumn(current string, channelOrigin bool) string {
	current = normalizeScrumColumn(current)
	if channelOrigin && current == "review" {
		return "review"
	}
	return "review"
}

func scrumChannelJobMetadata(metadata []byte, priorColumn string) []byte {
	var meta map[string]any
	if err := json.Unmarshal(metadata, &meta); err != nil || meta == nil {
		return metadata
	}
	meta["scrum_channel_origin"] = true
	if col := normalizeScrumColumn(priorColumn); col != "" {
		meta["scrum_return_column"] = col
	}
	out, err := json.Marshal(meta)
	if err != nil {
		return metadata
	}
	return out
}

func scrumReturnColumnFromMetadata(raw json.RawMessage) string {
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		return ""
	}
	col, _ := meta["scrum_return_column"].(string)
	return normalizeScrumColumn(col)
}
