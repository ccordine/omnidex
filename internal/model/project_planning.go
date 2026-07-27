package model

import "time"

type ProjectPlanningConfig struct {
	Model         string    `json:"model"`
	ReasoningMode string    `json:"reasoning_mode"`
	UpdatedAt     time.Time `json:"-"`
}

type ProjectPlanningMessage struct {
	ID        int64     `json:"id"`
	ProjectID int64     `json:"project_id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectPlanningDraft struct {
	ProjectID   int64      `json:"project_id"`
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Column      string     `json:"column"`
	Checklist   []string   `json:"checklist"`
	Status      string     `json:"status"`
	Source      string     `json:"source"`
	BatchID     string     `json:"batch_id"`
	CardID      string     `json:"card_id"`
	CreatedAt   time.Time  `json:"created_at"`
	AddedAt     *time.Time `json:"added_at,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type ProjectPlanningMessagePage struct {
	Messages     []ProjectPlanningMessage
	HasMore      bool
	NextBeforeID int64
}
