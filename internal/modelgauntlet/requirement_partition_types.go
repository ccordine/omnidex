package modelgauntlet

import (
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	RequirementPartitionReportSchemaV1   = "omnidex.model-gauntlet.requirement-partition-report.v1"
	RequirementPartitionResultSchemaV1   = "omnidex.model-gauntlet.requirement-partition-result.v1"
	RequirementPartitionPromptRendererV2 = "omnidex.structured-advisory-protocol.v1.requirement-partition.v2"
)

type RequirementPartitionCase struct {
	ID    string                                 `json:"id"`
	Input assemblyline.RequirementPartitionInput `json:"input"`
}

type RequirementPartitionLabel struct {
	CaseID        string   `json:"case_id"`
	FeatureQuotes []string `json:"feature_quotes"`
}

type RequirementPartitionConfig struct {
	StableModel    string
	ReasoningModel string
	ContextTokens  int
	KeepAlive      string
}

type RequirementPartitionConfigEvidence struct {
	StableModel    string `json:"stable_model"`
	ReasoningModel string `json:"reasoning_model"`
	ContextTokens  int    `json:"context_tokens"`
	KeepAlive      string `json:"keep_alive"`
	PromptRenderer string `json:"prompt_renderer"`
}

type RequirementPartitionPrediction struct {
	CaseID        string   `json:"case_id"`
	Variant       Variant  `json:"variant"`
	Valid         bool     `json:"valid"`
	FeatureQuotes []string `json:"feature_quotes,omitempty"`
	Error         string   `json:"error,omitempty"`
}

type RequirementPartitionReport struct {
	Schema      string                             `json:"schema"`
	StartedAt   time.Time                          `json:"started_at"`
	FinishedAt  time.Time                          `json:"finished_at"`
	Config      RequirementPartitionConfigEvidence `json:"config"`
	Cases       []RequirementPartitionCase         `json:"cases"`
	Calls       []CallEvidence                     `json:"calls"`
	Predictions []RequirementPartitionPrediction   `json:"predictions"`
}

type RequirementPartitionEvaluation struct {
	ReportSchema string                     `json:"report_schema"`
	Scores       map[Variant]VariantScore   `json:"scores"`
	Metrics      map[Variant]VariantMetrics `json:"metrics"`
}

type RequirementPartitionResult struct {
	Schema      string                         `json:"schema"`
	LabelSHA256 string                         `json:"label_sha256"`
	Report      RequirementPartitionReport     `json:"report"`
	Evaluation  RequirementPartitionEvaluation `json:"evaluation"`
}
