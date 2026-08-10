package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
)

const (
	ProjectionRelevanceSchemaV1 = "omnidex.private-projection-relevance.v1"
	CleanDeskMetricsSchemaV1    = "omnidex.clean-desk-metrics.v1"
)

type CriticalProjectionUse struct {
	ProjectionID  string                      `json:"projection_id"`
	Ref           ProjectionReferenceIdentity `json:"ref"`
	RequiredBytes int64                       `json:"required_bytes"`
}

type ProjectionRelevanceEvidence struct {
	Schema            string                        `json:"schema"`
	EpisodeSealSHA256 string                        `json:"episode_seal_sha256"`
	OracleSHA256      string                        `json:"oracle_sha256"`
	RelevantRefs      []ProjectionReferenceIdentity `json:"relevant_refs"`
	CriticalUses      []CriticalProjectionUse       `json:"critical_uses"`
}

type StationCallMetrics struct {
	CallID                  string                           `json:"call_id"`
	ProjectionID            string                           `json:"projection_id"`
	Budget                  StationBudget                    `json:"budget"`
	InputBytes              int64                            `json:"input_bytes"`
	InputTokens             int64                            `json:"input_tokens"`
	OutputBytes             int64                            `json:"output_bytes"`
	OutputTokens            int64                            `json:"output_tokens"`
	ResultStatus            cognitionpolicy.CallResultStatus `json:"result_status"`
	FailureCode             cognitionpolicy.CallFailureCode  `json:"failure_code,omitempty"`
	NativeUsagePresent      bool                             `json:"native_usage_present"`
	BudgetQualified         bool                             `json:"budget_qualified"`
	RelevantProjectedBytes  int64                            `json:"relevant_projected_bytes"`
	IrrelevantSelectedBytes int64                            `json:"irrelevant_selected_bytes"`
	MissingCriticalBytes    int64                            `json:"missing_critical_bytes"`
	MissingCriticalRefs     int                              `json:"missing_critical_refs"`
	ContextConcentration    float64                          `json:"context_concentration"`
	ConcentrationQualified  bool                             `json:"concentration_qualified"`
}

type CleanDeskMetrics struct {
	Schema                  string               `json:"schema"`
	EpisodeSealSHA256       string               `json:"episode_seal_sha256"`
	OracleSHA256            string               `json:"oracle_sha256"`
	Calls                   []StationCallMetrics `json:"calls"`
	TotalModelVisibleBytes  int64                `json:"total_model_visible_bytes"`
	TotalInputTokens        int64                `json:"total_input_tokens"`
	TotalOutputBytes        int64                `json:"total_output_bytes"`
	TotalOutputTokens       int64                `json:"total_output_tokens"`
	RelevantProjectedBytes  int64                `json:"relevant_projected_bytes"`
	IrrelevantSelectedBytes int64                `json:"irrelevant_selected_bytes"`
	MissingCriticalBytes    int64                `json:"missing_critical_bytes"`
	MissingCriticalRefs     int                  `json:"missing_critical_refs"`
	ContextConcentration    float64              `json:"context_concentration"`
	ConcentrationQualified  bool                 `json:"concentration_qualified"`
	NativeUsageComplete     bool                 `json:"native_usage_complete"`
	BudgetQualified         bool                 `json:"budget_qualified"`
}

func EvaluateCleanDesk(
	episode SealedEpisode,
	oracle OracleManifest,
	evidence ProjectionRelevanceEvidence,
) (CleanDeskMetrics, error) {
	if err := episode.Validate(); err != nil {
		return CleanDeskMetrics{}, err
	}
	if err := oracle.Validate(); err != nil {
		return CleanDeskMetrics{}, err
	}
	if episode.Manifest.Scenario.ID != oracle.ScenarioID ||
		episode.Manifest.Scenario.SHA256 != oracle.PublicSHA256 {
		return CleanDeskMetrics{}, fmt.Errorf("clean-desk evaluator received an oracle for another episode")
	}
	relevant, critical, err := validateProjectionRelevance(evidence, episode, oracle)
	if err != nil {
		return CleanDeskMetrics{}, err
	}
	metrics := CleanDeskMetrics{
		Schema: CleanDeskMetricsSchemaV1, EpisodeSealSHA256: episode.SealSHA256,
		OracleSHA256: oracle.OracleSHA256, Calls: make([]StationCallMetrics, 0, episode.Manifest.Resources.ModelCalls),
		NativeUsageComplete: true, BudgetQualified: true,
	}
	var pending *ProjectionTrace
	for _, entry := range episode.Manifest.Trace {
		switch entry.Kind {
		case TraceProjection:
			if pending != nil {
				return CleanDeskMetrics{}, fmt.Errorf("clean-desk trace replaced an unused projection")
			}
			projection := ProjectionTrace{}
			if err := decodeTracePayload(entry.Payload, &projection, "clean-desk projection"); err != nil {
				return CleanDeskMetrics{}, err
			}
			pending = &projection
		case TraceModelCall:
			call := ModelCallTrace{}
			if err := decodeTracePayload(entry.Payload, &call, "clean-desk model call"); err != nil {
				return CleanDeskMetrics{}, err
			}
			if pending == nil || pending.ProjectionID != call.ProjectionID {
				return CleanDeskMetrics{}, fmt.Errorf("clean-desk model call lacks its sealed projection")
			}
			measured, err := measureStationCall(entry.ID, *pending, call, relevant, critical[call.ProjectionID])
			if err != nil {
				return CleanDeskMetrics{}, err
			}
			metrics.Calls = append(metrics.Calls, measured)
			accumulateCleanDesk(&metrics, measured)
			pending = nil
		case TracePolicyDisposition:
			if pending == nil {
				return CleanDeskMetrics{}, fmt.Errorf("clean-desk policy disposition lacks its projection")
			}
			disposition := PolicyDispositionTrace{}
			if err := decodeTracePayload(entry.Payload, &disposition, "clean-desk policy disposition"); err != nil {
				return CleanDeskMetrics{}, err
			}
			if err := disposition.Validate(); err != nil || disposition.ProjectionID != pending.ProjectionID ||
				disposition.ProjectionSHA256 != pending.ProjectionSHA256 {
				return CleanDeskMetrics{}, fmt.Errorf("clean-desk policy disposition changed its projection")
			}
			pending = nil
		}
	}
	if len(metrics.Calls) == 0 {
		return CleanDeskMetrics{}, fmt.Errorf("clean-desk evaluation requires at least one sealed model call")
	}
	metrics.ContextConcentration = concentration(
		metrics.RelevantProjectedBytes, metrics.TotalModelVisibleBytes,
	)
	metrics.ConcentrationQualified = metrics.MissingCriticalRefs == 0 &&
		metrics.NativeUsageComplete && metrics.BudgetQualified
	if err := metrics.Validate(); err != nil {
		return CleanDeskMetrics{}, err
	}
	return metrics, nil
}

func measureStationCall(
	callID string,
	projection ProjectionTrace,
	call ModelCallTrace,
	relevant map[ProjectionReferenceIdentity]struct{},
	critical map[ProjectionReferenceIdentity]int64,
) (StationCallMetrics, error) {
	metric := StationCallMetrics{
		CallID: callID, ProjectionID: projection.ProjectionID, Budget: call.Budget,
		InputBytes: call.InputBytes, InputTokens: call.InputTokens,
		OutputBytes: call.OutputBytes, OutputTokens: call.OutputTokens,
		ResultStatus: call.ResultStatus, FailureCode: call.FailureCode,
		NativeUsagePresent: call.ProviderUsagePresent,
	}
	metric.BudgetQualified = callBudgetQualified(call)
	for _, item := range projection.Selected {
		if projectedReferenceIsRelevant(item, relevant) {
			metric.RelevantProjectedBytes += item.RenderedBytes
		} else {
			metric.IrrelevantSelectedBytes += item.RenderedBytes
		}
	}
	for ref, requiredBytes := range critical {
		if !projectionCarriesReference(projection.Selected, ref) {
			metric.MissingCriticalRefs++
			metric.MissingCriticalBytes += requiredBytes
		}
	}
	if metric.RelevantProjectedBytes+metric.IrrelevantSelectedBytes > metric.InputBytes {
		return StationCallMetrics{}, fmt.Errorf("selected projection bytes exceed model-visible input")
	}
	metric.ContextConcentration = concentration(metric.RelevantProjectedBytes, metric.InputBytes)
	metric.ConcentrationQualified = metric.MissingCriticalRefs == 0 &&
		metric.NativeUsagePresent && metric.BudgetQualified
	return metric, metric.Validate()
}

func projectedReferenceIsRelevant(
	item ProjectedReference,
	relevant map[ProjectionReferenceIdentity]struct{},
) bool {
	if _, exists := relevant[item.Ref]; exists {
		return true
	}
	for _, source := range item.SourceRefs {
		if _, exists := relevant[source]; exists {
			return true
		}
	}
	return false
}

func projectionCarriesReference(
	selected []ProjectedReference,
	expected ProjectionReferenceIdentity,
) bool {
	for _, item := range selected {
		if item.Ref == expected {
			return true
		}
		for _, source := range item.SourceRefs {
			if source == expected {
				return true
			}
		}
	}
	return false
}

func accumulateCleanDesk(total *CleanDeskMetrics, call StationCallMetrics) {
	total.TotalModelVisibleBytes += call.InputBytes
	total.TotalInputTokens += call.InputTokens
	total.TotalOutputBytes += call.OutputBytes
	total.TotalOutputTokens += call.OutputTokens
	total.RelevantProjectedBytes += call.RelevantProjectedBytes
	total.IrrelevantSelectedBytes += call.IrrelevantSelectedBytes
	total.MissingCriticalBytes += call.MissingCriticalBytes
	total.MissingCriticalRefs += call.MissingCriticalRefs
	total.NativeUsageComplete = total.NativeUsageComplete && call.NativeUsagePresent
	total.BudgetQualified = total.BudgetQualified && call.BudgetQualified
}

func concentration(relevant, total int64) float64 {
	if total == 0 {
		return 0
	}
	return float64(relevant) / float64(total)
}
