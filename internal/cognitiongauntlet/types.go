package cognitiongauntlet

import (
	"time"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/taskstate"
)

const (
	PublicManifestSchemaV1  = "omnidex.cognition-gauntlet-public.v1"
	OracleManifestSchemaV1  = "omnidex.cognition-gauntlet-oracle.v1"
	EpisodeManifestSchemaV2 = "omnidex.cognition-episode.v2"
	EpisodeSealSchemaV1     = "omnidex.cognition-episode-seal.v1"
	EvaluationSchemaV1      = "omnidex.cognition-evaluation.v1"
)

type Suite string

const (
	SuiteRetrieve Suite = "retrieve"
	SuiteRecall   Suite = "recall"
	SuiteUnlock   Suite = "unlock"
	SuiteMutate   Suite = "mutate"
	SuiteCombined Suite = "combined"
	SuiteTraverse Suite = "traverse"
	SuiteBind     Suite = "bind"
	SuiteRevise   Suite = "revise"
	SuiteOrder    Suite = "order"
	SuiteResume   Suite = "resume"
	SuiteScale    Suite = "scale"
	SuiteTransfer Suite = "transfer"
	SuiteRogue    Suite = "rogue"
)

type Difficulty struct {
	WorldSize             int     `json:"world_size"`
	RelevantArtifacts     int     `json:"relevant_artifacts"`
	SolutionDepth         int     `json:"solution_depth"`
	BranchingFactor       int     `json:"branching_factor"`
	DistractorRatio       float64 `json:"distractor_ratio"`
	SemanticAmbiguity     int     `json:"semantic_ambiguity"`
	DependencyCount       int     `json:"dependency_count"`
	DelayedFactCount      int     `json:"delayed_fact_count"`
	SimultaneousGoals     int     `json:"simultaneous_goals"`
	IrreversibleActions   int     `json:"irreversible_actions"`
	WorkingSetBudgetBytes int     `json:"working_set_budget_bytes"`
	ContextBudgetBytes    int     `json:"context_budget_bytes"`
	ToolBudget            int     `json:"tool_budget"`
	RestartCount          int     `json:"restart_count"`
}

type PublicManifest struct {
	Schema               string                `json:"schema"`
	Suite                Suite                 `json:"suite"`
	Scenario             cognition.ScenarioRef `json:"scenario"`
	FormatVersion        string                `json:"format_version"`
	SurfaceVersion       string                `json:"surface_version"`
	ActionCatalogVersion string                `json:"action_catalog_version"`
	ActionCatalogSHA256  string                `json:"action_catalog_sha256"`
	Goal                 string                `json:"goal"`
	Difficulty           Difficulty            `json:"difficulty"`
}

type OracleQuality string

const (
	OracleOptimal     OracleQuality = "optimal"
	OracleWitnessOnly OracleQuality = "witness_only"
)

type OracleManifest struct {
	Schema           string               `json:"schema"`
	ScenarioID       cognition.ScenarioID `json:"scenario_id"`
	PublicSHA256     string               `json:"public_sha256"`
	OracleSHA256     string               `json:"oracle_sha256"`
	GeneratorVersion string               `json:"generator_version"`
	Seed             uint64               `json:"seed"`
	Quality          OracleQuality        `json:"oracle_quality"`
	WitnessCost      int64                `json:"witness_cost"`
	OptimalCost      *int64               `json:"optimal_cost,omitempty"`
	LowerBound       int64                `json:"lower_bound"`
	TaskArchetype    string               `json:"task_archetype"`
}

type TraceKind string

const (
	TraceModelCall          TraceKind = "model_call"
	TracePolicyDisposition  TraceKind = "policy_disposition"
	TraceProjection         TraceKind = "context_projection"
	TraceObservation        TraceKind = "observation"
	TraceAction             TraceKind = "action"
	TraceLedger             TraceKind = "task_ledger"
	TraceWorkingSet         TraceKind = "working_set"
	TraceObligation         TraceKind = "obligation"
	TraceFailure            TraceKind = "failure"
	TraceRestart            TraceKind = "restart"
	TraceLease              TraceKind = "lease"
	TraceStaleRejection     TraceKind = "stale_rejection"
	TraceProviderBootstrap  TraceKind = "provider_bootstrap"
	TraceProviderActivation TraceKind = "provider_activation"
	TraceAblationEvidence   TraceKind = "ablation_evidence"
	TraceTerminal           TraceKind = "terminal"
)

type TraceEntry struct {
	Sequence      uint64                   `json:"sequence"`
	Kind          TraceKind                `json:"kind"`
	ID            string                   `json:"id"`
	Revision      *cognition.WorldRevision `json:"revision,omitempty"`
	Payload       taskstate.JSONObject     `json:"payload"`
	PayloadSHA256 string                   `json:"payload_sha256"`
}

type ModelRecord struct {
	Name                    string `json:"name"`
	Digest                  string `json:"digest"`
	Quantization            string `json:"quantization"`
	SamplingSHA256          string `json:"sampling_sha256"`
	ContextLimit            int    `json:"context_limit"`
	Hardware                string `json:"hardware"`
	HardwareAuthoritySource string `json:"hardware_authority_source"`
	Backend                 string `json:"backend"`
	BackendVersion          string `json:"backend_version"`
}

type Outcome struct {
	Terminal      bool   `json:"terminal"`
	GoalSatisfied bool   `json:"goal_satisfied"`
	PublicOutcome string `json:"public_outcome"`
	FailureCode   string `json:"failure_code,omitempty"`
}

type Resources struct {
	PolicyCallsConsumed           int   `json:"policy_calls_consumed"`
	ModelCalls                    int   `json:"model_calls"`
	ModelDecisions                int   `json:"model_decisions"`
	EnvironmentActions            int   `json:"environment_actions"`
	LowLevelTransitions           int   `json:"low_level_transitions"`
	ToolOperations                int   `json:"tool_operations"`
	SearchOperations              int   `json:"search_operations"`
	ReadOperations                int   `json:"read_operations"`
	InputTokens                   int64 `json:"input_tokens"`
	OutputTokens                  int64 `json:"output_tokens"`
	ContextBytes                  int64 `json:"context_bytes"`
	OutputBytes                   int64 `json:"output_bytes"`
	PeakContextBytes              int64 `json:"peak_context_bytes"`
	PeakWorkingSetBytes           int64 `json:"peak_working_set_bytes"`
	ProviderTotalNanoseconds      int64 `json:"provider_total_nanoseconds"`
	ProviderLoadNanoseconds       int64 `json:"provider_load_nanoseconds"`
	ProviderPromptEvalNanoseconds int64 `json:"provider_prompt_eval_nanoseconds"`
	ProviderEvalNanoseconds       int64 `json:"provider_eval_nanoseconds"`
	PolicyWallMilliseconds        int64 `json:"policy_wall_milliseconds"`
	WallMilliseconds              int64 `json:"wall_milliseconds"`
}

type MemoryMetrics struct {
	CriticalEvidenceAcquired int   `json:"critical_evidence_acquired"`
	CriticalEvidenceAtUse    int   `json:"critical_evidence_at_use"`
	ProjectionMisses         int   `json:"projection_misses"`
	StaleResidentBytes       int64 `json:"stale_resident_bytes"`
	IrrelevantResidentBytes  int64 `json:"irrelevant_resident_bytes"`
	ReleaseLatencyActions    int64 `json:"release_latency_actions"`
	Reacquisitions           int   `json:"reacquisitions"`
	Thrashes                 int   `json:"thrashes"`
}

type PlanningMetrics struct {
	ObligationsCreated   int `json:"obligations_created"`
	ObligationsCompleted int `json:"obligations_completed"`
	PlanGenerations      int `json:"plan_generations"`
	UnnecessarySubgoals  int `json:"unnecessary_subgoals"`
	DeadEndRevisits      int `json:"dead_end_revisits"`
	UnsupportedActions   int `json:"unsupported_actions"`
	InvalidActions       int `json:"invalid_actions"`
	Backtracks           int `json:"backtracks"`
}

type RecoveryMetrics struct {
	Restarts               int `json:"restarts"`
	RestorationMismatches  int `json:"restoration_mismatches"`
	DuplicateSuppressions  int `json:"duplicate_suppressions"`
	StaleAttemptRejections int `json:"stale_attempt_rejections"`
	ProjectionMismatches   int `json:"projection_mismatches"`
}

type EpisodeManifest struct {
	Schema                   string                  `json:"schema"`
	EpisodeID                cognition.EpisodeID     `json:"episode_id"`
	Scenario                 cognition.ScenarioRef   `json:"scenario"`
	PublicRunAuthoritySHA256 string                  `json:"public_run_authority_sha256"`
	Variant                  Variant                 `json:"variant"`
	OmnidexCommit            string                  `json:"omnidex_commit"`
	RuntimeVersion           string                  `json:"runtime_version"`
	LedgerSchemaVersion      string                  `json:"ledger_schema_version"`
	WorkingSetPolicyVersion  string                  `json:"working_set_policy_version"`
	ProjectionPolicyVersion  string                  `json:"projection_policy_version"`
	EpisodeStartedAt         time.Time               `json:"episode_started_at"`
	SealedAt                 time.Time               `json:"sealed_at"`
	RatGeneration            RatGeneration           `json:"rat_generation"`
	Model                    ModelRecord             `json:"model"`
	StationBudget            StationBudget           `json:"station_budget"`
	FinalRevision            cognition.WorldRevision `json:"final_revision"`
	Outcome                  Outcome                 `json:"outcome"`
	Trace                    []TraceEntry            `json:"trace"`
	TraceSHA256              string                  `json:"trace_sha256"`
	Resources                Resources               `json:"resources"`
	Memory                   MemoryMetrics           `json:"memory"`
	Planning                 PlanningMetrics         `json:"planning"`
	Recovery                 RecoveryMetrics         `json:"recovery"`
}

type SealedEpisode struct {
	Schema     string          `json:"schema"`
	Manifest   EpisodeManifest `json:"manifest"`
	SealSHA256 string          `json:"seal_sha256"`
}

type Evaluation struct {
	Schema                string                   `json:"schema"`
	EpisodeSealSHA256     string                   `json:"episode_seal_sha256"`
	OracleSHA256          string                   `json:"oracle_sha256"`
	Seed                  uint64                   `json:"seed"`
	EvaluatorVersion      string                   `json:"evaluator_version"`
	TaskArchetype         string                   `json:"task_archetype"`
	Quality               OracleQuality            `json:"oracle_quality"`
	GoalSuccess           bool                     `json:"goal_success"`
	ValidTerminalState    bool                     `json:"valid_terminal_state"`
	ActualDecisionCost    int64                    `json:"actual_decision_cost"`
	ReferenceDecisionCost int64                    `json:"reference_decision_cost"`
	CleanDesk             *CleanDeskMetrics        `json:"clean_desk,omitempty"`
	CausalAcquisition     *CausalAcquisitionReport `json:"causal_acquisition,omitempty"`
	Attribution           *FailureAttribution      `json:"attribution,omitempty"`
}

type MetricName string

const (
	MetricDecisionRegret  MetricName = "decision_regret"
	MetricWitnessOverhead MetricName = "witness_overhead"
)

type EfficiencyMetric struct {
	Name  MetricName `json:"name"`
	Ratio float64    `json:"ratio"`
}
