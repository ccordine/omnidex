package queue

import (
	"errors"
	"time"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/evidence"
)

const MaxJobHistoryPageSize = 100

var ErrInvalidJobHistoryRequest = errors.New("invalid job history request")

type JobHistoryStream string

const (
	JobHistoryGenerations JobHistoryStream = "generations"
	JobHistorySteps       JobHistoryStream = "steps"
	JobHistoryArtifacts   JobHistoryStream = "artifacts"
	JobHistoryEvidence    JobHistoryStream = "evidence"
	JobHistoryLLMCalls    JobHistoryStream = "llm_calls"
)

type JobHistoryRequest struct {
	Stream JobHistoryStream `json:"stream"`
	Limit  int              `json:"limit"`
	Cursor string           `json:"cursor,omitempty"`
}

type HistoricalStepReference struct {
	JobID                  int64  `json:"job_id"`
	StepID                 int64  `json:"step_id"`
	Generation             int64  `json:"generation"`
	SupersededAtGeneration *int64 `json:"superseded_at_generation,omitempty"`
}

type JobGenerationHistory struct {
	JobID                 int64     `json:"job_id"`
	Generation            int64     `json:"generation"`
	Purpose               string    `json:"purpose"`
	PredecessorGeneration *int64    `json:"predecessor_generation,omitempty"`
	BoundaryAction        string    `json:"boundary_action,omitempty"`
	Feedback              string    `json:"feedback,omitempty"`
	FeedbackSHA256        string    `json:"feedback_sha256,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
}

type HistoricalStep struct {
	HistoricalStepReference
	Action     string     `json:"action"`
	SortIndex  int        `json:"sort_index"`
	Status     string     `json:"status"`
	WorkerID   string     `json:"worker_id,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type HistoricalArtifact struct {
	Artifact artifacts.Envelope      `json:"artifact"`
	Step     HistoricalStepReference `json:"step"`
	cursorID int64
}

type HistoricalEvidence struct {
	Evidence evidence.Record         `json:"evidence"`
	Step     HistoricalStepReference `json:"step"`
}

type HistoricalLLMCall struct {
	Call LLMCallEvidence         `json:"call"`
	Step HistoricalStepReference `json:"step"`
}

type JobHistoryPage struct {
	JobID       int64                  `json:"job_id"`
	Stream      JobHistoryStream       `json:"stream"`
	Generations []JobGenerationHistory `json:"generations,omitempty"`
	Steps       []HistoricalStep       `json:"steps,omitempty"`
	Artifacts   []HistoricalArtifact   `json:"artifacts,omitempty"`
	Evidence    []HistoricalEvidence   `json:"evidence,omitempty"`
	LLMCalls    []HistoricalLLMCall    `json:"llm_calls,omitempty"`
	NextCursor  string                 `json:"next_cursor,omitempty"`
}
