package objectiveadvisory

import (
	"fmt"
	"reflect"
	"time"
)

// Configuration returns a detached copy of the immutable configuration used by
// this runtime. Consumers use it to bind a returned report to the runner that
// produced it.
func (runtime *Runtime) Configuration() Config {
	if runtime == nil {
		return Config{}
	}
	return cloneConfig(runtime.config)
}

// ValidateFor proves that a report is the exact bounded derivation permitted by
// the sent projection input and immutable runner configuration.
func (report Report) ValidateFor(input ProjectionInput, gap SemanticGap, config Config) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("objective advisory report configuration: %w", err)
	}
	expectedProjection, err := BuildProjection(input)
	if err != nil {
		return err
	}
	if err := gap.validateFor(expectedProjection); err != nil {
		return err
	}
	gapText, err := semanticGapText(gap)
	if err != nil {
		return err
	}
	if report.Mode != config.Mode {
		return fmt.Errorf("objective advisory report mode does not match its configured runner")
	}
	if report.TriggerID != TriggerPostGroundingObjective || report.TriggerVersion != TriggerVersionV1 ||
		report.SemanticGapSHA256 != digest(gapText) ||
		!reflect.DeepEqual(report.Projection, expectedProjection) {
		return fmt.Errorf("objective advisory report does not match its exact sent projection and trigger")
	}
	if report.Artifacts == nil || report.Chunks == nil || report.CandidateCapsules == nil ||
		report.ActiveCapsules == nil {
		return fmt.Errorf("objective advisory report collections must be explicit arrays")
	}
	expectedChunks, err := validateReportArtifacts(report, config)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(report.Chunks, expectedChunks) {
		return fmt.Errorf("objective advisory chunks are not the deterministic raw-artifact projection")
	}
	if err := validateReportCapsules(report, config); err != nil {
		return err
	}
	return validateReportMetrics(report, config)
}

func validateReportMetrics(report Report, config Config) error {
	want := Metrics{WallTime: report.Metrics.WallTime}
	if want.WallTime < 0 {
		return fmt.Errorf("objective advisory report wall time is negative")
	}
	for _, artifact := range report.Artifacts {
		want.RawBytes += artifact.RawBytes
		want.PromptTokens += artifact.PromptTokens
		want.OutputTokens += artifact.OutputTokens
	}
	want.AdvisoryCalls = len(report.Artifacts)
	want.ChunksProduced = len(report.Chunks)
	want.CandidateCapsules = len(report.CandidateCapsules)
	want.SelectedCapsules = len(report.ActiveCapsules)
	want.UnselectedChunks = len(report.Chunks) - len(report.ActiveCapsules)
	if len(report.CandidateCapsules) > 0 {
		want.PotentialCapsuleContentBytes = report.CandidateCapsules[0].ByteCost
		want.PotentialCapsuleContentTokens = report.CandidateCapsules[0].EstimatedTokens
	}
	if len(report.ActiveCapsules) > 0 {
		want.SelectedCapsuleContentBytes = report.ActiveCapsules[0].ByteCost
		want.SelectedCapsuleContentTokens = report.ActiveCapsules[0].EstimatedTokens
	}

	switch {
	case config.Mode == ModeOff:
		if report.ReductionStatus != StatusNotRun || report.ReductionError != "" {
			return fmt.Errorf("off objective advisory report has reduction activity")
		}
	case len(report.Chunks) == 0:
		if report.ReductionStatus != StatusFailed ||
			report.ReductionError != "no successful advisory content was eligible for reduction" {
			return fmt.Errorf("chunkless objective advisory report has invalid reduction status")
		}
	case report.ReductionStatus == StatusSucceeded:
		if report.ReductionError != "" {
			return fmt.Errorf("successful objective advisory reduction contains an error")
		}
		want.EmbeddingCalls = len(report.Chunks) + 1
	case report.ReductionStatus == StatusFailed:
		if err := validateText("reduction failure", report.ReductionError, maxFailureBytes, true); err != nil ||
			report.Metrics.EmbeddingCalls < 1 || report.Metrics.EmbeddingCalls > len(report.Chunks)+1 {
			return fmt.Errorf("failed objective advisory reduction has invalid failure accounting")
		}
		want.EmbeddingCalls = report.Metrics.EmbeddingCalls
	default:
		return fmt.Errorf("objective advisory report has invalid reduction status")
	}
	if !reflect.DeepEqual(report.Metrics, want) {
		return fmt.Errorf("objective advisory report metrics do not match its exact provenance graph")
	}
	return nil
}

func (runtime *Runtime) finishReport(
	report Report,
	input ProjectionInput,
	gap SemanticGap,
	started time.Time,
) (Report, error) {
	report.Metrics.WallTime = time.Since(started)
	if err := report.ValidateFor(input, gap, runtime.config); err != nil {
		return report, fmt.Errorf("validate owned objective advisory report: %w", err)
	}
	return report, nil
}
