package model

import (
	"encoding/json"
	"time"
)

type Project struct {
	ID           int64           `json:"id"`
	Name         string          `json:"name"`
	Location     string          `json:"location"`
	Description  string          `json:"description,omitempty"`
	RecipeID     string          `json:"recipe_id,omitempty"`
	Recipe       json.RawMessage `json:"recipe,omitempty"`
	ProjectState string          `json:"project_state,omitempty"`
	Settings     json.RawMessage `json:"settings,omitempty"`
	LastSeenAt   time.Time       `json:"last_seen_at"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type ProjectPatch struct {
	Name         *string          `json:"name,omitempty"`
	Location     *string          `json:"location,omitempty"`
	Description  *string          `json:"description,omitempty"`
	RecipeID     *string          `json:"recipe_id,omitempty"`
	Recipe       *json.RawMessage `json:"recipe,omitempty"`
	ProjectState *string          `json:"project_state,omitempty"`
	Settings     *json.RawMessage `json:"settings,omitempty"`
}
