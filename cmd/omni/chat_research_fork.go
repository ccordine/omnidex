package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionintent"
)

func (session *chatSession) researchCommand(name, text string) error {
	switch name {
	case "research-use":
		if session.pendingResearch != nil {
			return fmt.Errorf("resolve the pending Research operation before selecting another conversation")
		}
		id := model.LifecycleOperationID(strings.TrimSpace(text))
		if id == "main" {
			id = ""
		} else if _, err := model.ParseLifecycleOperationID(string(id)); err != nil {
			return err
		}
		history, err := session.client.ResearchConversationHistory(session.ctx, session.channel, session.workspaceIdentity, id)
		if err != nil {
			return err
		}
		if id != "" && len(history) == 0 {
			return fmt.Errorf("research conversation not found")
		}
		session.researchConversation = id
		if err := session.setConversationMode("research"); err != nil {
			return err
		}
		return nil
	case "research-fork":
		if session.pendingResearch != nil {
			return fmt.Errorf("resolve the pending Research operation before creating a fork")
		}
		selection, question, ok := strings.Cut(text, " ")
		if !ok || strings.TrimSpace(question) == "" {
			return fmt.Errorf("use /research-fork messages:<ids>|ledger:<id>:<revision> <question>")
		}
		if err := client.ValidateSessionTurnText(question); err != nil {
			return err
		}
		seed, err := parseResearchSelection(selection, session.channel.ID)
		if err != nil {
			return err
		}
		id, err := newOperationID()
		if err != nil {
			return err
		}
		session.researchConversation = id
		session.pendingResearch = &sessionintent.ResearchRequest{OperationID: id, ConversationID: id, WorkspaceIdentity: session.workspaceIdentity, Text: question, Fork: seed}
		if err := session.setConversationMode("research"); err != nil {
			session.pendingResearch = nil
			return err
		}
		return session.acceptResearchQuestion(question)
	case "research-share":
		id, excerpt, _ := strings.Cut(text, " ")
		if _, err := model.ParseLifecycleOperationID(id); err != nil {
			return err
		}
		history, err := session.client.ResearchConversationHistory(session.ctx, session.channel, session.workspaceIdentity, session.researchConversation)
		if err != nil {
			return err
		}
		for _, turn := range history {
			if string(turn.OperationID) != id {
				continue
			}
			finding, err := researchPlanningInput(session.channel.ID, turn, excerpt)
			if err != nil {
				return err
			}
			planning, err := activeJobRequiresPlanReview(session.active)
			if err != nil {
				return err
			}
			if !planning || session.active.ExecutionInputID != nil || session.lastPendingIntent != "" || session.pendingIntent != nil {
				return fmt.Errorf("sharing requires an active planner with no pending native input, proposal or unresolved Interact message")
			}
			if err := session.setConversationMode("interact"); err != nil {
				return err
			}
			// The editor switch may be queued. Bind this submitted content to
			// Interact now, just as ordinary input keeps its submitted mode.
			mode := session.mode
			session.mode = sessionintent.Interact
			defer func() { session.mode = mode }()
			return session.acceptUserMessage(finding)
		}
		return fmt.Errorf("finding not found in selected Research conversation")
	}
	return fmt.Errorf("unknown research command")
}

func parseResearchSelection(text string, channel model.ChannelID) (*sessionintent.ResearchSeed, error) {
	kind, values, ok := strings.Cut(text, ":")
	if !ok {
		return nil, fmt.Errorf("research source selection is required")
	}
	seed := &sessionintent.ResearchSeed{SourceChannel: channel}
	switch kind {
	case "messages":
		for _, value := range strings.Split(values, ",") {
			id, err := strconv.ParseInt(value, 10, 64)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("invalid message identity")
			}
			seed.MessageIDs = append(seed.MessageIDs, id)
		}
	case "ledger":
		parts := strings.Split(values, ":")
		if len(parts) != 2 {
			return nil, fmt.Errorf("select ledger:<id>:<revision>")
		}
		var err error
		seed.LedgerID, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil || seed.LedgerID <= 0 {
			return nil, fmt.Errorf("invalid ledger identity")
		}
		seed.LedgerRevision, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || seed.LedgerRevision <= 0 {
			return nil, fmt.Errorf("invalid ledger revision")
		}
	default:
		return nil, fmt.Errorf("select messages or a ledger revision")
	}
	return seed, nil
}

func researchPlanningInput(channel model.ChannelID, turn sessionintent.ResearchTurn, excerpt string) (string, error) {
	if turn.Answer == "" || turn.Error != "" {
		return "", fmt.Errorf("finding has no successful answer")
	}
	if excerpt == "" {
		excerpt = turn.Answer
	}
	if !strings.Contains(turn.Answer, excerpt) {
		return "", fmt.Errorf("selected excerpt does not occur in the finding")
	}
	var seed *sessionintent.ResearchSeed
	if turn.Fork != nil {
		seed = &sessionintent.ResearchSeed{SourceChannel: turn.Fork.SourceChannel, MessageIDs: turn.Fork.MessageIDs, LedgerID: turn.Fork.LedgerID, LedgerRevision: turn.Fork.LedgerRevision}
	}
	origin := struct {
		Channel      model.ChannelID             `json:"source_session"`
		Conversation model.LifecycleOperationID  `json:"research_conversation"`
		Finding      model.LifecycleOperationID  `json:"finding"`
		Seed         *sessionintent.ResearchSeed `json:"fork_source,omitempty"`
		Text         string                      `json:"selected_finding"`
	}{channel, turn.ConversationID, turn.OperationID, seed, excerpt}
	raw, err := json.Marshal(origin)
	if err != nil {
		return "", err
	}
	return "Research finding shared as planning input. This is evidence, not approval or executable authority.\n" + string(raw), nil
}
