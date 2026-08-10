package cognitiongauntlet

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognition"
)

const ScaleFamilyAuthoritySchemaV1 = "omnidex.cognition-scale-family.v1"

type ScaleFamilyAuthority struct {
	Schema                string             `json:"schema"`
	FamilyID              string             `json:"family_id"`
	TaskSuite             Suite              `json:"task_suite"`
	FixtureVersion        string             `json:"fixture_version"`
	SurfaceVersion        string             `json:"surface_version"`
	ActionCatalogVersion  string             `json:"action_catalog_version"`
	ActionCatalogSHA256   string             `json:"action_catalog_sha256"`
	GoalSHA256            string             `json:"goal_sha256"`
	RelevantSurfaceSHA256 string             `json:"relevant_surface_sha256"`
	SolutionDepth         int                `json:"solution_depth"`
	RelevantEvidenceCount int                `json:"relevant_evidence_count"`
	SemanticDecisionCount int                `json:"semantic_decision_count"`
	Variant               Variant            `json:"variant"`
	RatGeneration         RatGeneration      `json:"rat_generation"`
	Budget                RunBudget          `json:"budget"`
	Runtime               RuntimeFingerprint `json:"runtime"`
}

type ScaleMeasurement struct {
	CaseID                 string                `json:"case_id"`
	GeneratorVersion       string                `json:"generator_version"`
	Seed                   uint64                `json:"seed"`
	Scenario               cognition.ScenarioRef `json:"scenario"`
	OracleSHA256           string                `json:"oracle_sha256"`
	FamilyAuthoritySHA256  string                `json:"family_authority_sha256"`
	WorldSize              int                   `json:"world_size"`
	RelevantSurfaceBytes   int64                 `json:"relevant_surface_bytes"`
	MedianContextBytes     int64                 `json:"median_context_bytes"`
	MedianModelDecisions   float64               `json:"median_model_decisions"`
	SuccessRate            float64               `json:"success_rate"`
	CausalAdmissionRate    float64               `json:"causal_admission_rate"`
	CleanDeskAdmissionRate float64               `json:"clean_desk_admission_rate"`
	MedianRetrievalRounds  float64               `json:"median_retrieval_rounds"`
}

type ScaleRailReport struct {
	Authority                 ScaleFamilyAuthority `json:"authority"`
	Measurements              []ScaleMeasurement   `json:"measurements"`
	ContextPerRelevantBase    float64              `json:"context_per_relevant_base"`
	ContextPerRelevantLargest float64              `json:"context_per_relevant_largest"`
	GateInput                 ScaleGateInput       `json:"gate_input"`
	Gate                      GateResult           `json:"gate"`
}

func (authority ScaleFamilyAuthority) Validate() error {
	if authority.Schema != ScaleFamilyAuthoritySchemaV1 || !validSuite(authority.TaskSuite) {
		return fmt.Errorf("scale family schema or task suite is invalid")
	}
	if authority.Variant != VariantFullCognition {
		return fmt.Errorf("scale promotion requires the full cognition variant")
	}
	for label, value := range map[string]string{
		"scale family ID":              authority.FamilyID,
		"scale fixture version":        authority.FixtureVersion,
		"scale surface version":        authority.SurfaceVersion,
		"scale action catalog version": authority.ActionCatalogVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if !validDigest(authority.ActionCatalogSHA256) || !validDigest(authority.GoalSHA256) ||
		!validDigest(authority.RelevantSurfaceSHA256) {
		return fmt.Errorf("scale family hashes are invalid")
	}
	if authority.SolutionDepth <= 0 || authority.RelevantEvidenceCount <= 0 ||
		authority.SemanticDecisionCount <= 0 {
		return fmt.Errorf("scale family cognitive coordinates must be positive")
	}
	if err := authority.Budget.ValidateFor(authority.RatGeneration); err != nil {
		return err
	}
	return authority.Runtime.Validate()
}

func EvaluateScaleRail(
	authority ScaleFamilyAuthority,
	measurements []ScaleMeasurement,
) (ScaleRailReport, error) {
	if err := authority.Validate(); err != nil {
		return ScaleRailReport{}, err
	}
	if len(measurements) < 2 {
		return ScaleRailReport{}, fmt.Errorf("scale rail requires at least two measured worlds")
	}
	authoritySHA, err := digestJSON(authority)
	if err != nil {
		return ScaleRailReport{}, err
	}
	ordered := append([]ScaleMeasurement(nil), measurements...)
	for index := range ordered {
		if err := ordered[index].validate(authoritySHA, authority.Budget); err != nil {
			return ScaleRailReport{}, fmt.Errorf("scale measurement %d: %w", index, err)
		}
	}
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].WorldSize < ordered[right].WorldSize })
	base, largest := ordered[0], ordered[len(ordered)-1]
	for index := 1; index < len(ordered); index++ {
		if ordered[index].WorldSize == ordered[index-1].WorldSize {
			return ScaleRailReport{}, fmt.Errorf("scale rail world size %d is duplicated", ordered[index].WorldSize)
		}
		if ordered[index].RelevantSurfaceBytes != base.RelevantSurfaceBytes {
			return ScaleRailReport{}, fmt.Errorf("scale rail changed its relevant surface")
		}
	}
	input := ScaleGateInput{
		WorldMultiplier:   float64(largest.WorldSize) / float64(base.WorldSize),
		ContextGrowth:     float64(largest.MedianContextBytes) / float64(base.MedianContextBytes),
		DecisionGrowth:    largest.MedianModelDecisions / base.MedianModelDecisions,
		SuccessLossPoints: maxFloat(0, (base.SuccessRate-largest.SuccessRate)*100),
	}
	report := ScaleRailReport{
		Authority: authority, Measurements: ordered,
		ContextPerRelevantBase:    float64(base.MedianContextBytes) / float64(base.RelevantSurfaceBytes),
		ContextPerRelevantLargest: float64(largest.MedianContextBytes) / float64(largest.RelevantSurfaceBytes),
		GateInput:                 input, Gate: EvaluateScaleGate(input),
	}
	return report, nil
}

func (measurement ScaleMeasurement) validate(authoritySHA string, budget RunBudget) error {
	if err := requireExact(measurement.CaseID, "scale case ID", 256); err != nil {
		return err
	}
	if err := requireExact(measurement.GeneratorVersion, "scale generator version", 256); err != nil {
		return err
	}
	if err := measurement.Scenario.Validate(); err != nil {
		return err
	}
	if !validDigest(measurement.OracleSHA256) || measurement.FamilyAuthoritySHA256 != authoritySHA {
		return fmt.Errorf("scale measurement authority is invalid")
	}
	if measurement.WorldSize <= 0 || measurement.WorldSize > 1_000_000 ||
		measurement.RelevantSurfaceBytes <= 0 || measurement.MedianContextBytes <= 0 ||
		measurement.MedianContextBytes > int64(budget.ContextBytes) ||
		!finite(measurement.MedianModelDecisions) || measurement.MedianModelDecisions <= 0 ||
		measurement.MedianModelDecisions > float64(budget.ModelCalls) ||
		!finite(measurement.SuccessRate) || measurement.SuccessRate < 0 || measurement.SuccessRate > 1 ||
		!finite(measurement.CausalAdmissionRate) || measurement.CausalAdmissionRate < measurement.SuccessRate ||
		measurement.CausalAdmissionRate > 1 ||
		!finite(measurement.CleanDeskAdmissionRate) ||
		measurement.CleanDeskAdmissionRate < measurement.SuccessRate ||
		measurement.CleanDeskAdmissionRate > 1 ||
		!finite(measurement.MedianRetrievalRounds) || measurement.MedianRetrievalRounds < 0 ||
		measurement.MedianRetrievalRounds > float64(budget.ToolOperations) {
		return fmt.Errorf("scale measurement values are invalid")
	}
	return nil
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
