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
	ID                       string               `json:"id"`
	Title                    string               `json:"title"`
	Description              string               `json:"description"`
	Column                   string               `json:"column"`
	Checklist                []ScrumChecklistItem `json:"checklist"`
	RefFiles                 []string             `json:"ref_files"`
	Chat                     []ScrumChatMessage   `json:"chat"`
	ModelConfig              json.RawMessage      `json:"model_config,omitempty"`
	AgentConfig              json.RawMessage      `json:"agent_config,omitempty"`
	CardTicket               string               `json:"card_ticket,omitempty"`
	CardPrompt               string               `json:"card_prompt,omitempty"`
	RecipeID                 string               `json:"recipe_id,omitempty"`
	Recipe                   json.RawMessage      `json:"recipe,omitempty"`
	Tags                     []string             `json:"tags"`
	PlanningChat             []ScrumChatMessage   `json:"planning_chat"`
	TestCriteria             []ScrumChecklistItem `json:"test_criteria"`
	FlowMetrics              json.RawMessage      `json:"flow_metrics,omitempty"`
	Summary                  bool                 `json:"summary,omitempty"`
	ChecklistDone            int                  `json:"checklist_done,omitempty"`
	ChecklistTotal           int                  `json:"checklist_total,omitempty"`
	RefFileCount             int                  `json:"ref_file_count,omitempty"`
	ChatCount                int                  `json:"chat_count,omitempty"`
	PlanningChatCount        int                  `json:"planning_chat_count,omitempty"`
	TestCriteriaDone         int                  `json:"test_criteria_done,omitempty"`
	TestCriteriaTotal        int                  `json:"test_criteria_total,omitempty"`
	HasCardTicket            bool                 `json:"has_card_ticket,omitempty"`
	JobID                    string               `json:"job_id,omitempty"`
	ConsoleLog               string               `json:"console_log,omitempty"`
	PlayState                string               `json:"play_state,omitempty"`
	QueueOrder               int                  `json:"queue_order,omitempty"`
	BoardOrder               int                  `json:"board_order,omitempty"`
	SyncJobID                string               `json:"-"`
	AgentStreamChatCursor    int64                `json:"-"`
	AgentStreamConsoleCursor int64                `json:"-"`
	StepContextCursor        int64                `json:"-"`
	CreatedAt                string               `json:"created_at"`
	UpdatedAt                string               `json:"updated_at"`
}

type ScrumBoard struct {
	ID               string      `json:"id"`
	Name             string      `json:"name"`
	ProjectDirectory string      `json:"project_directory"`
	Columns          []string    `json:"columns"`
	Cards            []ScrumCard `json:"cards"`
	UpdatedAt        string      `json:"updated_at"`
}
