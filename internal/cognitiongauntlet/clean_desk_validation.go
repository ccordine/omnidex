package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func validateProjectionRelevance(
	evidence ProjectionRelevanceEvidence,
	episode SealedEpisode,
	oracle OracleManifest,
) (
	map[ProjectionReferenceIdentity]struct{},
	map[string]map[ProjectionReferenceIdentity]int64,
	error,
) {
	if evidence.Schema != ProjectionRelevanceSchemaV1 ||
		evidence.EpisodeSealSHA256 != episode.SealSHA256 ||
		evidence.OracleSHA256 != oracle.OracleSHA256 {
		return nil, nil, fmt.Errorf("private projection relevance authority is invalid")
	}
	if evidence.RelevantRefs == nil || evidence.CriticalUses == nil ||
		len(evidence.RelevantRefs) > maxEpisodeTraceEntries ||
		len(evidence.CriticalUses) > maxEpisodeTraceEntries {
		return nil, nil, fmt.Errorf("private projection relevance must be explicit and bounded")
	}
	relevant := make(map[ProjectionReferenceIdentity]struct{}, len(evidence.RelevantRefs))
	for index, ref := range evidence.RelevantRefs {
		if err := ref.Validate(); err != nil {
			return nil, nil, fmt.Errorf("relevant projection ref %d: %w", index+1, err)
		}
		if _, duplicate := relevant[ref]; duplicate {
			return nil, nil, fmt.Errorf("relevant projection ref %d is duplicated", index+1)
		}
		relevant[ref] = struct{}{}
	}
	sealedProjections := make(map[string]struct{}, episode.Manifest.Resources.ModelCalls)
	for _, entry := range episode.Manifest.Trace {
		if entry.Kind == TraceProjection {
			sealedProjections[entry.ID] = struct{}{}
		}
	}
	critical := make(map[string]map[ProjectionReferenceIdentity]int64)
	for index, use := range evidence.CriticalUses {
		if err := requireExact(use.ProjectionID, "critical projection ID", 512); err != nil {
			return nil, nil, err
		}
		if err := use.Ref.Validate(); err != nil {
			return nil, nil, fmt.Errorf("critical projection use %d: %w", index+1, err)
		}
		if use.RequiredBytes <= 0 || use.RequiredBytes > cognition.MaxObservationBytes {
			return nil, nil, fmt.Errorf("critical projection use %d byte count is invalid", index+1)
		}
		if _, exists := relevant[use.Ref]; !exists {
			return nil, nil, fmt.Errorf("critical projection use %d lacks private relevance authority", index+1)
		}
		if _, exists := sealedProjections[use.ProjectionID]; !exists {
			return nil, nil, fmt.Errorf("critical projection use %d cites an unsealed projection", index+1)
		}
		uses := critical[use.ProjectionID]
		if uses == nil {
			uses = make(map[ProjectionReferenceIdentity]int64)
			critical[use.ProjectionID] = uses
		}
		if _, duplicate := uses[use.Ref]; duplicate {
			return nil, nil, fmt.Errorf("critical projection use %d is duplicated", index+1)
		}
		uses[use.Ref] = use.RequiredBytes
	}
	return relevant, critical, nil
}

func (metric StationCallMetrics) Validate() error {
	if requireExact(metric.CallID, "clean-desk call ID", 512) != nil ||
		requireExact(metric.ProjectionID, "clean-desk projection ID", 512) != nil {
		return fmt.Errorf("clean-desk station call identity is invalid")
	}
	if err := metric.Budget.Validate(); err != nil {
		return err
	}
	if metric.InputBytes <= 0 || metric.InputTokens <= 0 || metric.OutputBytes < 0 ||
		metric.OutputTokens < 0 || metric.RelevantProjectedBytes < 0 ||
		metric.IrrelevantSelectedBytes < 0 || metric.MissingCriticalBytes < 0 ||
		metric.InputBytes > int64(metric.Budget.MaxInputBytes) ||
		metric.InputTokens > int64(metric.Budget.MaxInputTokens) ||
		metric.OutputBytes > int64(metric.Budget.MaxOutputBytes) ||
		metric.OutputTokens > int64(metric.Budget.MaxOutputTokens) ||
		(metric.OutputBytes == 0) != (metric.OutputTokens == 0) ||
		metric.MissingCriticalRefs < 0 || metric.RelevantProjectedBytes+metric.IrrelevantSelectedBytes > metric.InputBytes ||
		!finite(metric.ContextConcentration) || metric.ContextConcentration < 0 ||
		metric.ContextConcentration > 1 ||
		metric.ContextConcentration != concentration(metric.RelevantProjectedBytes, metric.InputBytes) ||
		metric.ConcentrationQualified != (metric.MissingCriticalRefs == 0) ||
		(metric.MissingCriticalBytes == 0) != (metric.MissingCriticalRefs == 0) {
		return fmt.Errorf("clean-desk station call metrics are inconsistent")
	}
	return nil
}

func (metrics CleanDeskMetrics) Validate() error {
	if metrics.Schema != CleanDeskMetricsSchemaV1 || !validDigest(metrics.EpisodeSealSHA256) ||
		!validDigest(metrics.OracleSHA256) || len(metrics.Calls) == 0 ||
		len(metrics.Calls) > maxEpisodeTraceEntries {
		return fmt.Errorf("clean-desk metric authority is invalid")
	}
	var total CleanDeskMetrics
	seenCalls := make(map[string]struct{}, len(metrics.Calls))
	for index, call := range metrics.Calls {
		if err := call.Validate(); err != nil {
			return fmt.Errorf("clean-desk call %d: %w", index+1, err)
		}
		if _, duplicate := seenCalls[call.CallID]; duplicate {
			return fmt.Errorf("clean-desk call %q is duplicated", call.CallID)
		}
		seenCalls[call.CallID] = struct{}{}
		accumulateCleanDesk(&total, call)
	}
	if metrics.TotalModelVisibleBytes != total.TotalModelVisibleBytes ||
		metrics.TotalInputTokens != total.TotalInputTokens || metrics.TotalOutputBytes != total.TotalOutputBytes ||
		metrics.TotalOutputTokens != total.TotalOutputTokens ||
		metrics.RelevantProjectedBytes != total.RelevantProjectedBytes ||
		metrics.IrrelevantSelectedBytes != total.IrrelevantSelectedBytes ||
		metrics.MissingCriticalBytes != total.MissingCriticalBytes ||
		metrics.MissingCriticalRefs != total.MissingCriticalRefs ||
		metrics.ContextConcentration != concentration(metrics.RelevantProjectedBytes, metrics.TotalModelVisibleBytes) ||
		metrics.ConcentrationQualified != (metrics.MissingCriticalRefs == 0) {
		return fmt.Errorf("aggregate clean-desk metrics do not match their sealed calls")
	}
	return nil
}
