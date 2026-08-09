package modelgauntlet

import (
	"context"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	CapabilityRelationReportSchemaV1   = "omnidex.model-gauntlet.capability-relation-report.v1"
	CapabilityRelationResultSchemaV1   = "omnidex.model-gauntlet.capability-relation-result.v1"
	CapabilityRelationPromptRendererV2 = "omnidex.capability-relation-sandwich-renderer.v2"
	CapabilityRelationPromptRendererV3 = "omnidex.capability-relation-sandwich-renderer.v3"
	CapabilityRelationPromptRendererV4 = "omnidex.capability-relation-sandwich-renderer.v4"
	CapabilityRelationPromptRendererV6 = "omnidex.structured-advisory-protocol.v1.capability-relation.v6"
)

const (
	maxStructuredTokens   = 128
	maxLensTokens         = 64
	maxDeliberationTokens = 1024
	maxDeliberationBytes  = 16 * 1024
)

type Variant string

const (
	VariantDirect            Variant = "direct"
	VariantDeliberated       Variant = "deliberated"
	VariantPerSplitAdvisory  Variant = "per_split_advisory"
	VariantFinalPassAdvisory Variant = "final_pass_advisory"
)

type CallStage string

const (
	StageDirect       CallStage = "direct"
	StageBriefing     CallStage = "briefing"
	StageDeliberation CallStage = "deliberation"
	StageSynthesis    CallStage = "synthesis"
)

type CapabilityRelationCase struct {
	ID    string                               `json:"id"`
	Input assemblyline.CapabilityRelationInput `json:"input"`
}

type CapabilityRelationLabel struct {
	CaseID   string                          `json:"case_id"`
	Relation assemblyline.CapabilityRelation `json:"relation"`
}

type CapabilityRelationConfig struct {
	StableModel    string
	ReasoningModel string
	ContextTokens  int
	KeepAlive      string
}

type GenerateRequest struct {
	CaseID          string         `json:"case_id"`
	Repetition      int            `json:"repetition,omitempty"`
	Operation       string         `json:"operation,omitempty"`
	Variant         Variant        `json:"variant"`
	Stage           CallStage      `json:"stage"`
	Model           string         `json:"model"`
	SystemPrompt    string         `json:"system_prompt"`
	UserPrompt      string         `json:"user_prompt"`
	ResponseSchema  map[string]any `json:"response_schema,omitempty"`
	Think           bool           `json:"think"`
	MaxOutputTokens int            `json:"max_output_tokens"`
	ContextTokens   int            `json:"context_tokens"`
	KeepAlive       string         `json:"keep_alive"`
}

type GenerateResponse struct {
	Model               string `json:"model,omitempty"`
	ModelDigest         string `json:"model_digest,omitempty"`
	Quantization        string `json:"quantization,omitempty"`
	ParameterSize       string `json:"parameter_size,omitempty"`
	Thinking            string `json:"thinking,omitempty"`
	Content             string `json:"content,omitempty"`
	TotalDuration       int64  `json:"total_duration_ns,omitempty"`
	LoadDuration        int64  `json:"load_duration_ns,omitempty"`
	PromptEvalCount     int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration  int64  `json:"prompt_eval_duration_ns,omitempty"`
	EvalCount           int    `json:"eval_count,omitempty"`
	EvalDuration        int64  `json:"eval_duration_ns,omitempty"`
	AllocatedBytes      int64  `json:"allocated_bytes,omitempty"`
	VRAMBytes           int64  `json:"vram_bytes,omitempty"`
	RunnerContextTokens int    `json:"runner_context_tokens,omitempty"`
}

type Generator interface {
	Generate(context.Context, GenerateRequest) (GenerateResponse, error)
}

type CallEvidence struct {
	PromptSHA256 string           `json:"prompt_sha256"`
	StartedAt    time.Time        `json:"started_at"`
	FinishedAt   time.Time        `json:"finished_at"`
	Request      GenerateRequest  `json:"request"`
	Response     GenerateResponse `json:"response"`
	Error        string           `json:"error,omitempty"`
}

type CapabilityRelationPrediction struct {
	CaseID   string                          `json:"case_id"`
	Variant  Variant                         `json:"variant"`
	Valid    bool                            `json:"valid"`
	Relation assemblyline.CapabilityRelation `json:"relation,omitempty"`
	Error    string                          `json:"error,omitempty"`
}

type CapabilityRelationReport struct {
	Schema      string                           `json:"schema"`
	StartedAt   time.Time                        `json:"started_at"`
	FinishedAt  time.Time                        `json:"finished_at"`
	Config      CapabilityRelationConfigEvidence `json:"config"`
	Cases       []CapabilityRelationCase         `json:"cases"`
	Calls       []CallEvidence                   `json:"calls"`
	Predictions []CapabilityRelationPrediction   `json:"predictions"`
}

type CapabilityRelationConfigEvidence struct {
	StableModel    string `json:"stable_model"`
	ReasoningModel string `json:"reasoning_model"`
	ContextTokens  int    `json:"context_tokens"`
	KeepAlive      string `json:"keep_alive"`
	PromptRenderer string `json:"prompt_renderer"`
}

type VariantScore struct {
	Total   int `json:"total"`
	Valid   int `json:"valid"`
	Correct int `json:"correct"`
}

type CapabilityRelationEvaluation struct {
	ReportSchema string                     `json:"report_schema"`
	Scores       map[Variant]VariantScore   `json:"scores"`
	Metrics      map[Variant]VariantMetrics `json:"metrics"`
}

type VariantMetrics struct {
	Calls             int   `json:"calls"`
	TotalDuration     int64 `json:"total_duration_ns"`
	LoadDuration      int64 `json:"load_duration_ns"`
	PromptTokens      int   `json:"prompt_tokens"`
	EvalTokens        int   `json:"eval_tokens"`
	MaxAllocatedBytes int64 `json:"max_allocated_bytes"`
	MaxVRAMBytes      int64 `json:"max_vram_bytes"`
}

type CapabilityRelationResult struct {
	Schema      string                       `json:"schema"`
	LabelSHA256 string                       `json:"label_sha256"`
	Report      CapabilityRelationReport     `json:"report"`
	Evaluation  CapabilityRelationEvaluation `json:"evaluation"`
}
