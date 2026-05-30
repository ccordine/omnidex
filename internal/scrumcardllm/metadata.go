package scrumcardllm

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

const (
	JobSource           = "scrum_card_llm"
	ActionTagsSuggest   = "tags_suggest"
	ActionCardTicket    = "card_ticket"
	MetadataProjectID   = "project_id"
	MetadataCardID      = "scrum_card_id"
	MetadataAction      = "action"
	MetadataCoachModel  = "coach_model"
	MetadataTicketModel = "ticket_model"
	MetadataTicketReq   = "ticket_request"
)

type TicketRequest struct {
	Prompt       string `json:"prompt"`
	CardPrompt   string `json:"card_prompt"`
	Ticket       string `json:"ticket"`
	Iterate      bool   `json:"iterate"`
	IterateNotes string `json:"iterate_notes"`
}

type ParsedMetadata struct {
	ProjectID   int64
	CardID      string
	Action      string
	CoachModel  string
	TicketModel string
	TicketReq   TicketRequest
}

func JobMetadata(projectID int64, cardID, action, coachModel, ticketModel string, ticketReq TicketRequest) ([]byte, error) {
	cardID = strings.TrimSpace(cardID)
	action = strings.TrimSpace(action)
	if projectID <= 0 {
		return nil, fmt.Errorf("project_id is required")
	}
	if cardID == "" {
		return nil, fmt.Errorf("scrum_card_id is required")
	}
	if action != ActionTagsSuggest && action != ActionCardTicket {
		return nil, fmt.Errorf("unsupported scrum card llm action %q", action)
	}
	payload := map[string]any{
		"source":            JobSource,
		MetadataProjectID:   projectID,
		MetadataCardID:      cardID,
		MetadataAction:      action,
		MetadataCoachModel:  strings.TrimSpace(coachModel),
		MetadataTicketModel: strings.TrimSpace(ticketModel),
	}
	if action == ActionCardTicket {
		payload[MetadataTicketReq] = ticketReq
	}
	return json.Marshal(payload)
}

func IsJobMetadata(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	return strings.TrimSpace(stringFromAny(payload["source"])) == JobSource
}

func ParseMetadata(raw json.RawMessage) (ParsedMetadata, error) {
	if len(raw) == 0 {
		return ParsedMetadata{}, fmt.Errorf("job metadata is empty")
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ParsedMetadata{}, fmt.Errorf("parse job metadata: %w", err)
	}
	if strings.TrimSpace(stringFromAny(payload["source"])) != JobSource {
		return ParsedMetadata{}, fmt.Errorf("not a scrum card llm job")
	}
	out := ParsedMetadata{
		CardID:      strings.TrimSpace(stringFromAny(payload[MetadataCardID])),
		Action:      strings.TrimSpace(stringFromAny(payload[MetadataAction])),
		CoachModel:  strings.TrimSpace(stringFromAny(payload[MetadataCoachModel])),
		TicketModel: strings.TrimSpace(stringFromAny(payload[MetadataTicketModel])),
	}
	switch v := payload[MetadataProjectID].(type) {
	case float64:
		out.ProjectID = int64(v)
	case int64:
		out.ProjectID = v
	case string:
		out.ProjectID, _ = strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	}
	if out.ProjectID <= 0 {
		return ParsedMetadata{}, fmt.Errorf("project_id is required")
	}
	if out.CardID == "" {
		return ParsedMetadata{}, fmt.Errorf("scrum_card_id is required")
	}
	if out.Action != ActionTagsSuggest && out.Action != ActionCardTicket {
		return ParsedMetadata{}, fmt.Errorf("unsupported action %q", out.Action)
	}
	if rawReq, ok := payload[MetadataTicketReq]; ok && rawReq != nil {
		blob, err := json.Marshal(rawReq)
		if err != nil {
			return ParsedMetadata{}, err
		}
		_ = json.Unmarshal(blob, &out.TicketReq)
	}
	return out, nil
}

func Pipeline() string {
	return model.PipelineScrumCardLLM
}

func stringFromAny(v any) string {
	if v == nil {
		return ""
	}
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}
