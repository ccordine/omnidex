package queue

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
)

// scrumChannelCardProjection is the closed immutable HTTP receipt stored for a
// Scrum channel operation. Internal cursors and retired configuration never
// enter the receipt.
type scrumChannelCardProjection struct {
	ID                  string          `json:"id"`
	ProjectID           int64           `json:"project_id"`
	Title               string          `json:"title"`
	Description         string          `json:"description"`
	Column              string          `json:"column"`
	Checklist           json.RawMessage `json:"checklist"`
	RefFiles            json.RawMessage `json:"ref_files"`
	CardTicket          string          `json:"card_ticket"`
	CardPrompt          string          `json:"card_prompt"`
	Tags                json.RawMessage `json:"tags"`
	TestCriteria        json.RawMessage `json:"test_criteria"`
	FlowMetrics         json.RawMessage `json:"flow_metrics"`
	JobID               string          `json:"job_id"`
	PlayState           string          `json:"play_state"`
	QueueOrder          int             `json:"queue_order"`
	BoardOrder          int             `json:"board_order"`
	ChannelMessageCount int64           `json:"channel_message_count"`
	ChannelContentBytes int64           `json:"channel_content_bytes"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

func newScrumChannelCardProjection(card DBScrumCard) (scrumChannelCardProjection, error) {
	flowMetrics, err := canonicalScrumFlowMetricsForRevision(card.FlowMetrics, card.UpdatedAt)
	if err != nil {
		return scrumChannelCardProjection{}, err
	}
	return scrumChannelCardProjection{
		ID: card.ID, ProjectID: card.ProjectID, Title: card.Title,
		Description: card.Description, Column: card.Column,
		Checklist: card.Checklist, RefFiles: card.RefFiles,
		CardTicket: card.CardTicket, CardPrompt: card.CardPrompt,
		Tags: card.Tags, TestCriteria: card.TestCriteria,
		FlowMetrics: flowMetrics, JobID: card.JobID,
		PlayState: card.PlayState, QueueOrder: card.QueueOrder, BoardOrder: card.BoardOrder,
		ChannelMessageCount: card.ChannelMessageCount, ChannelContentBytes: card.ChannelContentBytes,
		CreatedAt: card.CreatedAt.UTC(), UpdatedAt: card.UpdatedAt.UTC(),
	}, nil
}

func (projection scrumChannelCardProjection) card() DBScrumCard {
	return DBScrumCard{
		ID: projection.ID, ProjectID: projection.ProjectID, Title: projection.Title,
		Description: projection.Description, Column: projection.Column,
		Checklist: projection.Checklist, RefFiles: projection.RefFiles,
		CardTicket: projection.CardTicket, CardPrompt: projection.CardPrompt,
		Tags: projection.Tags, TestCriteria: projection.TestCriteria,
		FlowMetrics: projection.FlowMetrics, JobID: projection.JobID,
		PlayState: projection.PlayState, QueueOrder: projection.QueueOrder,
		BoardOrder: projection.BoardOrder, ChannelMessageCount: projection.ChannelMessageCount,
		ChannelContentBytes: projection.ChannelContentBytes,
		CreatedAt:           projection.CreatedAt, UpdatedAt: projection.UpdatedAt,
	}
}

func encodeScrumChannelCardProjection(card DBScrumCard) ([]byte, error) {
	projection, err := newScrumChannelCardProjection(card)
	if err != nil {
		return nil, fmt.Errorf("validate closed Scrum channel flow metrics: %w", err)
	}
	encoded, err := exactjson.Canonical(projection)
	if err != nil {
		return nil, fmt.Errorf("encode closed Scrum channel card projection: %w", err)
	}
	return encoded, nil
}

func decodeScrumChannelCardProjection(raw []byte) (DBScrumCard, error) {
	if err := exactjson.ValidateObject(raw, scrumChannelCardProjection{}, "Scrum channel card projection"); err != nil {
		return DBScrumCard{}, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return DBScrumCard{}, fmt.Errorf("decode Scrum channel card projection fields: %w", err)
	}
	if len(fields) != 20 {
		return DBScrumCard{}, fmt.Errorf("Scrum channel card projection has %d fields, expected 20", len(fields))
	}
	var projection scrumChannelCardProjection
	if err := json.Unmarshal(raw, &projection); err != nil {
		return DBScrumCard{}, fmt.Errorf("decode Scrum channel card projection: %w", err)
	}
	if _, err := canonicalScrumFlowMetricsForRevision(projection.FlowMetrics, projection.UpdatedAt); err != nil {
		return DBScrumCard{}, fmt.Errorf("decode closed Scrum channel flow metrics: %w", err)
	}
	return projection.card(), nil
}

func canonicalScrumFlowMetrics(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("Scrum flow metrics raw object is required")
	}
	if err := exactjson.ValidateObject(raw, ScrumFlowMetrics{}, "Scrum flow metrics"); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode Scrum flow metric fields: %w", err)
	}
	if len(fields) == 0 {
		return json.RawMessage(`{}`), nil
	}
	for _, name := range []string{
		"assigned_returns", "review_bounces", "regression_count", "play_runs",
		"channel_messages", "conversation_chars", "incomplete_score", "completion_status", "signals",
	} {
		value, present := fields[name]
		if !present {
			return nil, fmt.Errorf("Scrum flow metrics requires exact field %s", name)
		}
		if string(value) == "null" {
			return nil, fmt.Errorf("Scrum flow metric %s must not be null", name)
		}
	}
	for _, name := range []string{"last_play_outcome", "column", "updated_at"} {
		if value, present := fields[name]; present {
			switch string(value) {
			case `""`:
				return nil, fmt.Errorf("Scrum flow metric %s must be absent instead of explicitly empty", name)
			case "null":
				return nil, fmt.Errorf("Scrum flow metric %s must be absent instead of null", name)
			}
		}
	}
	var metrics ScrumFlowMetrics
	if err := json.Unmarshal(raw, &metrics); err != nil {
		return nil, fmt.Errorf("decode Scrum flow metrics: %w", err)
	}
	if metrics.Signals == nil {
		return nil, fmt.Errorf("Scrum flow metrics signals must be one explicit array")
	}
	if err := validateScrumFlowMetrics(metrics); err != nil {
		return nil, err
	}
	encoded, err := exactjson.Canonical(metrics)
	if err != nil {
		return nil, fmt.Errorf("encode Scrum flow metrics: %w", err)
	}
	return encoded, nil
}
