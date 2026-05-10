package evidence

import (
	"errors"
	"strings"
	"time"
)

const (
	KindFileExcerpt   = "file_excerpt"
	KindCommandOutput = "command_output"
	KindTestResult    = "test_result"
	KindWebPage       = "web_page"
	KindSearchResult  = "search_result"
	KindMemoryExcerpt = "memory_excerpt"
	KindGeneratedDiff = "generated_diff"
	KindModelJudgment = "model_judgment"
)

type Record struct {
	ID             int64          `json:"id,omitempty"`
	JobID          int64          `json:"job_id,omitempty"`
	StepID         int64          `json:"step_id,omitempty"`
	Kind           string         `json:"kind"`
	SourceType     string         `json:"source_type,omitempty"`
	SourceRef      string         `json:"source_ref,omitempty"`
	ToolName       string         `json:"tool_name,omitempty"`
	Command        string         `json:"command,omitempty"`
	FilePaths      []string       `json:"file_paths,omitempty"`
	Excerpt        string         `json:"excerpt,omitempty"`
	Summary        string         `json:"summary,omitempty"`
	Hash           string         `json:"hash,omitempty"`
	Confidence     float64        `json:"confidence,omitempty"`
	SupportsClaims []string       `json:"supports_claims,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
	CreatedAt      time.Time      `json:"created_at,omitempty"`
}

func (r Record) Validate() error {
	if strings.TrimSpace(r.Kind) == "" {
		return errors.New("evidence kind is required")
	}
	return nil
}
