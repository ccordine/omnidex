package scrumcardllm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
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
	PlanningMode bool   `json:"planning_mode"`
}

type ParsedMetadata struct {
	ProjectID   int64
	CardID      string
	Action      string
	CoachModel  string
	TicketModel string
	TicketReq   TicketRequest
}

type JobReference struct {
	ProjectID int64
	CardID    string
}

type metadataPayload struct {
	Source         string         `json:"source"`
	ProjectID      int64          `json:"project_id"`
	CardID         string         `json:"scrum_card_id"`
	Action         string         `json:"action"`
	CoachModel     string         `json:"coach_model,omitempty"`
	TicketModel    string         `json:"ticket_model,omitempty"`
	TicketRequest  *TicketRequest `json:"ticket_request,omitempty"`
	TelemetryRunID string         `json:"telemetry_run_id,omitempty"`
}

func JobMetadata(projectID int64, cardID, action, coachModel, ticketModel string, ticketReq TicketRequest) ([]byte, error) {
	payload := metadataPayload{
		Source:    JobSource,
		ProjectID: projectID,
		CardID:    strings.TrimSpace(cardID),
		Action:    strings.TrimSpace(action),
	}
	switch payload.Action {
	case ActionTagsSuggest:
		payload.CoachModel = strings.TrimSpace(coachModel)
		if payload.CoachModel == "" {
			return nil, fmt.Errorf("coach_model is required for %s", ActionTagsSuggest)
		}
	case ActionCardTicket:
		payload.TicketModel = strings.TrimSpace(ticketModel)
		if payload.TicketModel == "" {
			return nil, fmt.Errorf("ticket_model is required for %s", ActionCardTicket)
		}
		validated, err := validateTicketRequest(ticketReq)
		if err != nil {
			return nil, err
		}
		payload.TicketRequest = &validated
	default:
		return nil, fmt.Errorf("unsupported scrum card llm action %q", payload.Action)
	}
	if payload.ProjectID <= 0 {
		return nil, fmt.Errorf("project_id is required")
	}
	if payload.CardID == "" {
		return nil, fmt.Errorf("scrum_card_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum card LLM metadata: %w", err)
	}
	return encoded, nil
}

func IsJobMetadata(raw json.RawMessage) bool {
	if len(bytes.TrimSpace(raw)) == 0 {
		return false
	}
	var envelope struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return false
	}
	return envelope.Source == JobSource
}

func ParseMetadata(raw json.RawMessage) (ParsedMetadata, error) {
	payload, err := decodeMetadataPayload(raw)
	if err != nil {
		return ParsedMetadata{}, err
	}
	reference, err := validateJobReference(payload)
	if err != nil {
		return ParsedMetadata{}, err
	}
	payload.Action = strings.TrimSpace(payload.Action)
	payload.CoachModel = strings.TrimSpace(payload.CoachModel)
	payload.TicketModel = strings.TrimSpace(payload.TicketModel)
	out := ParsedMetadata{ProjectID: reference.ProjectID, CardID: reference.CardID, Action: payload.Action}
	switch payload.Action {
	case ActionTagsSuggest:
		if payload.CoachModel == "" {
			return ParsedMetadata{}, fmt.Errorf("coach_model is required for %s", ActionTagsSuggest)
		}
		if payload.TicketModel != "" || payload.TicketRequest != nil {
			return ParsedMetadata{}, fmt.Errorf("%s metadata cannot contain ticket fields", ActionTagsSuggest)
		}
		out.CoachModel = payload.CoachModel
	case ActionCardTicket:
		if payload.TicketModel == "" {
			return ParsedMetadata{}, fmt.Errorf("ticket_model is required for %s", ActionCardTicket)
		}
		if payload.CoachModel != "" || payload.TicketRequest == nil {
			return ParsedMetadata{}, fmt.Errorf("%s metadata requires ticket_request and cannot contain coach_model", ActionCardTicket)
		}
		request, err := validateTicketRequest(*payload.TicketRequest)
		if err != nil {
			return ParsedMetadata{}, err
		}
		out.TicketModel = payload.TicketModel
		out.TicketReq = request
	default:
		return ParsedMetadata{}, fmt.Errorf("unsupported action %q", payload.Action)
	}
	return out, nil
}

// ParseJobReference validates the durable Scrum identity without requiring an
// action to have been executable. Completion reconciliation must still run for
// a job that failed because its action-specific metadata was invalid.
func ParseJobReference(raw json.RawMessage) (JobReference, error) {
	payload, err := decodeMetadataPayload(raw)
	if err != nil {
		return JobReference{}, err
	}
	return validateJobReference(payload)
}

func decodeMetadataPayload(raw json.RawMessage) (metadataPayload, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return metadataPayload{}, fmt.Errorf("job metadata is empty")
	}
	var payload metadataPayload
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return metadataPayload{}, fmt.Errorf("parse Scrum card LLM metadata: %w", err)
	}
	if err := requireMetadataEOF(decoder); err != nil {
		return metadataPayload{}, err
	}
	return payload, nil
}

func validateJobReference(payload metadataPayload) (JobReference, error) {
	payload.CardID = strings.TrimSpace(payload.CardID)
	if payload.Source != JobSource {
		return JobReference{}, fmt.Errorf("not a Scrum card LLM job")
	}
	if payload.ProjectID <= 0 {
		return JobReference{}, fmt.Errorf("project_id is required")
	}
	if payload.CardID == "" {
		return JobReference{}, fmt.Errorf("scrum_card_id is required")
	}
	return JobReference{ProjectID: payload.ProjectID, CardID: payload.CardID}, nil
}

func Pipeline() string {
	return model.PipelineScrumCardLLM
}

func validateTicketRequest(request TicketRequest) (TicketRequest, error) {
	request.Prompt = strings.TrimSpace(request.Prompt)
	request.CardPrompt = strings.TrimSpace(request.CardPrompt)
	request.Ticket = strings.TrimSpace(request.Ticket)
	request.IterateNotes = strings.TrimSpace(request.IterateNotes)
	for _, field := range []struct {
		label string
		value string
	}{
		{label: "prompt", value: request.Prompt},
		{label: "card_prompt", value: request.CardPrompt},
		{label: "ticket", value: request.Ticket},
		{label: "iterate_notes", value: request.IterateNotes},
	} {
		if len(field.value) > 16_000 {
			return TicketRequest{}, fmt.Errorf("ticket_request.%s exceeds 16000 characters", field.label)
		}
	}
	if request.Iterate && request.Ticket == "" {
		return TicketRequest{}, fmt.Errorf("ticket_request.ticket is required when iterate is true")
	}
	if !request.Iterate && request.IterateNotes != "" {
		return TicketRequest{}, fmt.Errorf("ticket_request.iterate_notes requires iterate=true")
	}
	return request, nil
}

func requireMetadataEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("Scrum card LLM metadata contains trailing JSON")
		}
		return fmt.Errorf("Scrum card LLM metadata contains trailing data: %w", err)
	}
	return nil
}
