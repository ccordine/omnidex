package modelgauntlet

import "time"

const (
	CompleteRequirementCasesSchemaV1      = "omnidex.model-gauntlet.complete-requirement-cases.v1"
	CompleteRequirementLabelsSchemaV1     = "omnidex.model-gauntlet.complete-requirement-labels.v1"
	CompleteRequirementReportSchemaV1     = "omnidex.model-gauntlet.complete-requirement-report.v1"
	CompleteRequirementResultSchemaV1     = "omnidex.model-gauntlet.complete-requirement-result.v1"
	CompleteRequirementRendererV3         = "omnidex.requirement-final-advisory.v2.final-output-4096"
	minimumCompleteRequirementCases       = 50
	minimumCompleteRequirementRepeats     = 2
	maxFinalRequirementDeliberationTokens = 4096
)

type CompleteRequirementCase struct {
	ID         string `json:"id"`
	SourceText string `json:"source_text"`
}

type CompleteRequirementLabel struct {
	CaseID        string   `json:"case_id"`
	FeatureQuotes []string `json:"feature_quotes"`
}

type CompleteRequirementConfig struct {
	StableModel    string
	ReasoningModel string
	ContextTokens  int
	KeepAlive      string
	Repetitions    int
	CasesSHA256    string
	HardwareClass  string
	Backend        string
}

type CompleteRequirementConfigEvidence struct {
	StableModel                     string `json:"stable_model"`
	ReasoningModel                  string `json:"reasoning_model"`
	ContextTokens                   int    `json:"context_tokens"`
	KeepAlive                       string `json:"keep_alive"`
	Repetitions                     int    `json:"repetitions"`
	CasesSHA256                     string `json:"cases_sha256"`
	HardwareClass                   string `json:"hardware_class"`
	Backend                         string `json:"backend"`
	PromptRenderer                  string `json:"prompt_renderer"`
	StructuredMaxOutputTokens       int    `json:"structured_max_output_tokens"`
	PerSplitAdvisoryMaxOutputTokens int    `json:"per_split_advisory_max_output_tokens"`
	FinalAdvisoryMaxOutputTokens    int    `json:"final_advisory_max_output_tokens"`
}

type CompleteRequirementPrediction struct {
	CaseID        string   `json:"case_id"`
	Repetition    int      `json:"repetition"`
	Variant       Variant  `json:"variant"`
	Valid         bool     `json:"valid"`
	FeatureQuotes []string `json:"feature_quotes,omitempty"`
	Error         string   `json:"error,omitempty"`
}

type CompleteRequirementReport struct {
	Schema      string                            `json:"schema"`
	StartedAt   time.Time                         `json:"started_at"`
	FinishedAt  time.Time                         `json:"finished_at"`
	Config      CompleteRequirementConfigEvidence `json:"config"`
	Cases       []CompleteRequirementCase         `json:"cases"`
	Calls       []CallEvidence                    `json:"calls"`
	Predictions []CompleteRequirementPrediction   `json:"predictions"`
}

type PairedTransitions struct {
	DirectPassAssistedPass int `json:"direct_pass_assisted_pass"`
	DirectPassAssistedFail int `json:"direct_pass_assisted_fail"`
	DirectFailAssistedPass int `json:"direct_fail_assisted_pass"`
	DirectFailAssistedFail int `json:"direct_fail_assisted_fail"`
}

type VariantStability struct {
	Cases    int `json:"cases"`
	Stable   int `json:"stable"`
	Unstable int `json:"unstable"`
}

type CompleteRequirementPromotion struct {
	Eligible bool     `json:"eligible"`
	Reasons  []string `json:"reasons"`
}

type CompleteRequirementEvaluation struct {
	ReportSchema string                        `json:"report_schema"`
	Scores       map[Variant]VariantScore      `json:"scores"`
	Metrics      map[Variant]VariantMetrics    `json:"metrics"`
	Transitions  map[Variant]PairedTransitions `json:"transitions"`
	Stability    map[Variant]VariantStability  `json:"stability"`
	Promotion    CompleteRequirementPromotion  `json:"promotion"`
}

type CompleteRequirementResult struct {
	Schema      string                        `json:"schema"`
	LabelSHA256 string                        `json:"label_sha256"`
	Report      CompleteRequirementReport     `json:"report"`
	Evaluation  CompleteRequirementEvaluation `json:"evaluation"`
}
