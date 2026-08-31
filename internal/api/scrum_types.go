package api

import "encoding/json"

var scrumColumns = []string{"backlog", "ready", "assigned", "in_progress", "review", "blocked", "error", "done"}

type ScrumChecklistItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type ScrumChatMessage struct {
	ID          string `json:"id,omitempty"`
	Role        string `json:"role"`
	Content     string `json:"content"`
	CreatedAt   string `json:"created_at"`
	Status      string `json:"status,omitempty"`
	OperationID string `json:"operation_id,omitempty"`
}

type ScrumCard struct {
	ID                     string               `json:"id"`
	Title                  string               `json:"title"`
	Description            string               `json:"description"`
	Column                 string               `json:"column"`
	Checklist              []ScrumChecklistItem `json:"checklist"`
	RefFiles               []string             `json:"ref_files"`
	Chat                   []ScrumChatMessage   `json:"chat"`
	CardTicket             string               `json:"card_ticket,omitempty"`
	CardPrompt             string               `json:"card_prompt,omitempty"`
	Tags                   []string             `json:"tags"`
	TestCriteria           []ScrumChecklistItem `json:"test_criteria"`
	FlowMetrics            json.RawMessage      `json:"flow_metrics,omitempty"`
	Summary                bool                 `json:"summary,omitempty"`
	ChecklistDone          int                  `json:"checklist_done"`
	ChecklistTotal         int                  `json:"checklist_total"`
	RefFileCount           int                  `json:"ref_file_count"`
	ChatCount              int64                `json:"chat_count"`
	ChannelBeforeCursor    string               `json:"channel_before_cursor"`
	ChannelHasMore         bool                 `json:"channel_has_more"`
	ChannelContentBytes    int64                `json:"-"`
	TestCriteriaDone       int                  `json:"test_criteria_done"`
	TestCriteriaTotal      int                  `json:"test_criteria_total"`
	HasCardTicket          bool                 `json:"has_card_ticket"`
	JobID                  string               `json:"job_id,omitempty"`
	PlayState              string               `json:"play_state,omitempty"`
	QueueOrder             int                  `json:"queue_order,omitempty"`
	BoardOrder             int                  `json:"board_order"`
	PendingChannelMessages []ScrumChatMessage   `json:"-"`
	CreatedAt              string               `json:"created_at"`
	UpdatedAt              string               `json:"updated_at"`
}

type ScrumBoard struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	ProjectDirectory string      `json:"project_directory"`
	Columns          []string    `json:"columns"`
	Cards            []ScrumCard `json:"cards"`
	UpdatedAt        string      `json:"updated_at"`
}
