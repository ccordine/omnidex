package modelgauntlet

import (
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	RepositoryRetrievalCasesSchemaV1    = "omnidex.model-gauntlet.repository-retrieval-cases.v1"
	RepositoryRetrievalLabelsSchemaV1   = "omnidex.model-gauntlet.repository-retrieval-labels.v1"
	RepositoryRetrievalReportSchemaV1   = "omnidex.model-gauntlet.repository-retrieval-report.v1"
	RepositoryRetrievalResultSchemaV1   = "omnidex.model-gauntlet.repository-retrieval-result.v1"
	RepositoryRetrievalPromptRendererV1 = "omnidex.structured-advisory-protocol.v1.repository-retrieval.v1"
)

type RepositoryRetrievalCase struct {
	ID    string                                `json:"id"`
	Input assemblyline.RepositoryRetrievalInput `json:"input"`
}

type RepositoryRetrievalLabel struct {
	CaseID     string                                    `json:"case_id"`
	Operation  assemblyline.RepositoryRetrievalOperation `json:"operation"`
	QueryQuote string                                    `json:"query_quote"`
}

type RepositoryRetrievalConfig struct {
	StableModel    string
	ReasoningModel string
	ContextTokens  int
	KeepAlive      string
}

type RepositoryRetrievalConfigEvidence struct {
	StableModel    string `json:"stable_model"`
	ReasoningModel string `json:"reasoning_model"`
	ContextTokens  int    `json:"context_tokens"`
	KeepAlive      string `json:"keep_alive"`
	PromptRenderer string `json:"prompt_renderer"`
}

type RepositoryRetrievalPrediction struct {
	CaseID     string                                    `json:"case_id"`
	Variant    Variant                                   `json:"variant"`
	Valid      bool                                      `json:"valid"`
	Operation  assemblyline.RepositoryRetrievalOperation `json:"operation,omitempty"`
	QueryQuote string                                    `json:"query_quote,omitempty"`
	Error      string                                    `json:"error,omitempty"`
}

type RepositoryRetrievalReport struct {
	Schema      string                            `json:"schema"`
	StartedAt   time.Time                         `json:"started_at"`
	FinishedAt  time.Time                         `json:"finished_at"`
	Config      RepositoryRetrievalConfigEvidence `json:"config"`
	Cases       []RepositoryRetrievalCase         `json:"cases"`
	Calls       []CallEvidence                    `json:"calls"`
	Predictions []RepositoryRetrievalPrediction   `json:"predictions"`
}

type RepositoryRetrievalEvaluation struct {
	ReportSchema string                     `json:"report_schema"`
	Scores       map[Variant]VariantScore   `json:"scores"`
	Metrics      map[Variant]VariantMetrics `json:"metrics"`
}

type RepositoryRetrievalResult struct {
	Schema      string                        `json:"schema"`
	LabelSHA256 string                        `json:"label_sha256"`
	Report      RepositoryRetrievalReport     `json:"report"`
	Evaluation  RepositoryRetrievalEvaluation `json:"evaluation"`
}
