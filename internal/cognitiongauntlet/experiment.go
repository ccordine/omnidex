package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	PairedRunAuthoritySchemaV1  = "omnidex.cognition-paired-run.v1"
	RunBudgetSchemaStructuralV1 = "omnidex.cognition-run-budget.structural.v1"
	RunBudgetSchemaRawV2        = "omnidex.cognition-run-budget.raw-input.v2"
)

type Variant string

const (
	VariantDeterministicOracle Variant = "deterministic_oracle"
	VariantRawObservation      Variant = "raw_observation"
	VariantFullTranscript      Variant = "full_transcript"
	VariantTranscriptCompacted Variant = "transcript_compacted"
	VariantTaskLedger          Variant = "task_ledger"
	VariantLedgerWorkingSet    Variant = "ledger_working_set"
	VariantLedgerProjection    Variant = "ledger_context_projection"
	VariantFullCognition       Variant = "full_cognition"
	VariantOracleEvidence      Variant = "oracle_evidence_packet"
	VariantRawShell            Variant = "raw_shell"
)

type RunBudget struct {
	Schema             string         `json:"schema"`
	ContextBytes       int            `json:"context_bytes"`
	WorkingSetBytes    int            `json:"working_set_bytes"`
	RuntimeCycles      int            `json:"runtime_cycles"`
	ModelCalls         int            `json:"model_calls"`
	EnvironmentActions int            `json:"environment_actions"`
	ToolOperations     int            `json:"tool_operations"`
	Station            StationBudget  `json:"station"`
	Decision           DecisionBudget `json:"decision"`
}

type DecisionBudget struct {
	MaxEvidenceRefs        int `json:"max_evidence_refs"`
	MaxActionArguments     int `json:"max_action_arguments"`
	MaxLedgerProposals     int `json:"max_ledger_proposals"`
	MaxAttentionRequests   int `json:"max_attention_requests"`
	MaxExpectedEffectBytes int `json:"max_expected_effect_bytes"`
}

type PairedRunAuthority struct {
	Schema               string                `json:"schema"`
	CaseID               string                `json:"case_id"`
	Suite                Suite                 `json:"suite"`
	FixtureVersion       string                `json:"fixture_version"`
	GeneratorVersion     string                `json:"generator_version"`
	Seed                 uint64                `json:"seed"`
	Scenario             cognition.ScenarioRef `json:"scenario"`
	OracleSHA256         string                `json:"oracle_sha256"`
	SurfaceVersion       string                `json:"surface_version"`
	ActionCatalogVersion string                `json:"action_catalog_version"`
	ActionCatalogSHA256  string                `json:"action_catalog_sha256"`
	RatGeneration        RatGeneration         `json:"rat_generation"`
	Budget               RunBudget             `json:"budget"`
	Runtime              RuntimeFingerprint    `json:"runtime"`
	Repetition           int                   `json:"repetition"`
}

type VariantResult struct {
	Authority         PairedRunAuthority `json:"authority"`
	Variant           Variant            `json:"variant"`
	EpisodeSealSHA256 string             `json:"episode_seal_sha256"`
}

func (authority PairedRunAuthority) Validate() error {
	if authority.Schema != PairedRunAuthoritySchemaV1 {
		return fmt.Errorf("paired cognition run schema is invalid")
	}
	if err := requireExact(authority.CaseID, "paired cognition case ID", 256); err != nil {
		return err
	}
	if !validSuite(authority.Suite) {
		return fmt.Errorf("paired cognition suite is invalid")
	}
	for label, value := range map[string]string{
		"paired cognition fixture version":        authority.FixtureVersion,
		"paired cognition generator version":      authority.GeneratorVersion,
		"paired cognition surface version":        authority.SurfaceVersion,
		"paired cognition action catalog version": authority.ActionCatalogVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if err := authority.Scenario.Validate(); err != nil {
		return err
	}
	if !validDigest(authority.OracleSHA256) || !validDigest(authority.ActionCatalogSHA256) {
		return fmt.Errorf("paired cognition oracle or action catalog identity is invalid")
	}
	if err := authority.Budget.ValidateFor(authority.RatGeneration); err != nil {
		return err
	}
	if err := authority.Runtime.Validate(); err != nil {
		return err
	}
	if authority.Repetition <= 0 || authority.Repetition > 10_000 {
		return fmt.Errorf("paired cognition repetition is invalid")
	}
	return nil
}

func (budget RunBudget) Validate() error {
	if budget.Schema != RunBudgetSchemaStructuralV1 && budget.Schema != RunBudgetSchemaRawV2 {
		return fmt.Errorf("paired cognition run-budget schema is invalid")
	}
	if budget.ContextBytes <= 0 || budget.WorkingSetBytes <= 0 || budget.RuntimeCycles <= 0 ||
		budget.ModelCalls <= 0 ||
		budget.EnvironmentActions <= 0 || budget.ToolOperations <= 0 {
		return fmt.Errorf("paired cognition budgets must all be positive")
	}
	if err := budget.Station.Validate(); err != nil {
		return err
	}
	if err := budget.Decision.Validate(); err != nil {
		return err
	}
	if budget.ContextBytes > 64*1024*1024 || budget.WorkingSetBytes > 64*1024*1024 ||
		budget.RuntimeCycles > 1_000_000 || budget.ModelCalls > 1_000_000 || budget.EnvironmentActions > 1_000_000 ||
		budget.ToolOperations > 1_000_000 {
		return fmt.Errorf("paired cognition budget exceeds registered limits")
	}
	if budget.Schema == RunBudgetSchemaRawV2 &&
		budget.Station.MaxInputTokens != budget.Station.MaxInputBytes+
			llm.MaxRawInputSpecialTokenReserve {
		return fmt.Errorf("raw cognition station input ceiling is not code-derived")
	}
	return nil
}

// NewExecutableRunBudgetV2 supersedes a structural benchmark budget with the
// exact raw-provider input authority. The caller never supplies the derived
// token ceiling persisted in serious run configuration and preregistration.
func NewExecutableRunBudgetV2(
	structural RunBudget,
	sampling cognitionpolicy.SamplingIdentity,
) (RunBudget, error) {
	if structural.Schema != RunBudgetSchemaStructuralV1 {
		return RunBudget{}, fmt.Errorf("executable cognition budget requires structural v1 input")
	}
	if err := structural.Validate(); err != nil {
		return RunBudget{}, err
	}
	if err := sampling.Validate(); err != nil {
		return RunBudget{}, err
	}
	if structural.ContextBytes != sampling.ContextCeilingBytes ||
		structural.Station.MaxInputBytes > sampling.ContextCeilingBytes ||
		structural.Station.MaxOutputTokens != sampling.MaxOutputTokens {
		return RunBudget{}, fmt.Errorf("structural cognition budget changed frozen sampling ceilings")
	}
	result := structural
	result.Schema = RunBudgetSchemaRawV2
	result.Station.MaxInputTokens = result.Station.MaxInputBytes + sampling.InputSpecialTokenReserve
	if err := result.Validate(); err != nil {
		return RunBudget{}, err
	}
	return result, nil
}

func (budget DecisionBudget) Validate() error {
	runtime := cognition.RuntimeBudget{
		RemainingPolicyCalls: 1,
		MaxInputBytes:        1, MaxInputTokens: 1,
		MaxOutputBytes: 1, MaxOutputTokens: 1,
		MaxEvidenceRefs:        budget.MaxEvidenceRefs,
		MaxActionArguments:     budget.MaxActionArguments,
		MaxLedgerProposals:     budget.MaxLedgerProposals,
		MaxAttentionRequests:   budget.MaxAttentionRequests,
		MaxExpectedEffectBytes: budget.MaxExpectedEffectBytes,
	}
	if err := runtime.Validate(); err != nil {
		return fmt.Errorf("paired cognition decision budget: %w", err)
	}
	return nil
}

func (budget RunBudget) RuntimeBudget() (cognition.RuntimeBudget, error) {
	if err := budget.Validate(); err != nil {
		return cognition.RuntimeBudget{}, err
	}
	runtime := cognition.RuntimeBudget{
		RemainingPolicyCalls:   uint32(budget.ModelCalls),
		MaxInputBytes:          budget.Station.MaxInputBytes,
		MaxInputTokens:         budget.Station.MaxInputTokens,
		MaxOutputBytes:         budget.Station.MaxOutputBytes,
		MaxOutputTokens:        budget.Station.MaxOutputTokens,
		MaxEvidenceRefs:        budget.Decision.MaxEvidenceRefs,
		MaxActionArguments:     budget.Decision.MaxActionArguments,
		MaxLedgerProposals:     budget.Decision.MaxLedgerProposals,
		MaxAttentionRequests:   budget.Decision.MaxAttentionRequests,
		MaxExpectedEffectBytes: budget.Decision.MaxExpectedEffectBytes,
	}
	return runtime, runtime.Validate()
}

func (budget RunBudget) ValidateFor(generation RatGeneration) error {
	if err := budget.Validate(); err != nil {
		return err
	}
	if err := generation.Validate(); err != nil {
		return err
	}
	if budget.Schema != RunBudgetSchemaRawV2 {
		return fmt.Errorf("structural cognition budget is not executable under the raw provider contract")
	}
	if budget.ContextBytes != generation.Fixed.ContextCeilingBytes {
		return fmt.Errorf("cognition run changed its frozen context ceiling")
	}
	if budget.Station.MaxInputBytes > budget.ContextBytes {
		return fmt.Errorf("cognition station budget exceeds its frozen brain or context ceiling")
	}
	brain, err := productionBrain(generation, budget.Station.MaxOutputTokens)
	if err != nil {
		return fmt.Errorf("cognition station sampling authority: %w", err)
	}
	if budget.Station.MaxOutputTokens > brain.Sampling.MaxOutputTokens ||
		budget.Station.MaxInputTokens != budget.Station.MaxInputBytes+
			brain.Sampling.InputSpecialTokenReserve {
		return fmt.Errorf("cognition station output exceeds its frozen sampling ceiling")
	}
	runtime, err := budget.RuntimeBudget()
	if err != nil {
		return err
	}
	if err := cognitionpolicy.ValidateRuntimeBudget(brain, runtime); err != nil {
		return fmt.Errorf("cognition station cannot fit its exact raw inference authority: %w", err)
	}
	return nil
}

func (result VariantResult) Validate() error {
	if err := result.Authority.Validate(); err != nil {
		return err
	}
	if !validVariant(result.Variant) || !validDigest(result.EpisodeSealSHA256) {
		return fmt.Errorf("paired cognition variant or episode seal identity is invalid")
	}
	return nil
}

func RequirePairedVariants(left, right VariantResult) error {
	if err := left.Validate(); err != nil {
		return fmt.Errorf("left paired cognition result: %w", err)
	}
	if err := right.Validate(); err != nil {
		return fmt.Errorf("right paired cognition result: %w", err)
	}
	leftAuthority, err := digestJSON(left.Authority)
	if err != nil {
		return fmt.Errorf("hash left paired cognition authority: %w", err)
	}
	rightAuthority, err := digestJSON(right.Authority)
	if err != nil {
		return fmt.Errorf("hash right paired cognition authority: %w", err)
	}
	if leftAuthority != rightAuthority {
		return fmt.Errorf("paired cognition variants changed case, seed authority, brain, or budgets")
	}
	if left.Variant == right.Variant {
		return fmt.Errorf("paired cognition comparison requires two distinct variants")
	}
	return nil
}

func validVariant(variant Variant) bool {
	switch variant {
	case VariantDeterministicOracle, VariantRawObservation, VariantFullTranscript, VariantTranscriptCompacted,
		VariantTaskLedger, VariantLedgerWorkingSet, VariantLedgerProjection,
		VariantFullCognition, VariantOracleEvidence, VariantRawShell:
		return true
	default:
		return false
	}
}
