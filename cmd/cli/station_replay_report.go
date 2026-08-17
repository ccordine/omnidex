package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/worker"
)

const stationReplayReportSchema = "omnidex.station-replay-report.v1"
const stationConvergenceReportSchema = "omnidex.guided-typescript-convergence-report.v1"
const stationCurrentContractReplayReportSchema = "omnidex.station-current-contract-replay-report.v1"
const stationSpecificationConvergenceReportSchema = "omnidex.application-job-specification-convergence-report.v1"

type stationReplayReportHeader struct {
	Type                        string                   `json:"type"`
	Schema                      string                   `json:"schema"`
	CreatedAt                   time.Time                `json:"created_at"`
	SourceCallOpening           queue.StationCallOpening `json:"source_call_opening"`
	SourceCallWireRequestBase64 string                   `json:"source_call_wire_request_base64"`
	SourceGapOpening            queue.StationGapOpening  `json:"source_gap_opening"`
	Models                      []string                 `json:"models"`
	Timeout                     string                   `json:"timeout"`
	GuidanceModel               string                   `json:"guidance_model,omitempty"`
}

type stationConvergenceIterationEvidence struct {
	Number                                int                          `json:"number"`
	GuidanceProviderResponseCaptureBase64 string                       `json:"guidance_provider_response_capture_base64"`
	GuidanceProviderIdentityEvidence      llm.ProviderIdentityEvidence `json:"guidance_provider_identity_evidence"`
	ExecutorProviderResponseCaptureBase64 string                       `json:"executor_provider_response_capture_base64"`
	ExecutorProviderIdentityEvidence      llm.ProviderIdentityEvidence `json:"executor_provider_identity_evidence"`
}

type stationConvergenceReportRun struct {
	Type        string                                `json:"type"`
	StartedAt   time.Time                             `json:"started_at"`
	FinishedAt  time.Time                             `json:"finished_at"`
	Status      string                                `json:"status"`
	Error       string                                `json:"error,omitempty"`
	Convergence worker.ExactTypeScriptConvergence     `json:"convergence"`
	Evidence    []stationConvergenceIterationEvidence `json:"iteration_evidence"`
}

type stationReplayReportRun struct {
	Type                          string                       `json:"type"`
	StartedAt                     time.Time                    `json:"started_at"`
	FinishedAt                    time.Time                    `json:"finished_at"`
	Status                        string                       `json:"status"`
	Error                         string                       `json:"error,omitempty"`
	Replay                        worker.ExactStationReplay    `json:"replay"`
	ProviderResponseCaptureBase64 string                       `json:"provider_response_capture_base64,omitempty"`
	ProviderIdentityEvidence      llm.ProviderIdentityEvidence `json:"provider_identity_evidence,omitempty"`
}

type stationSpecificationConvergenceReportRun struct {
	Type        string                                             `json:"type"`
	StartedAt   time.Time                                          `json:"started_at"`
	FinishedAt  time.Time                                          `json:"finished_at"`
	Status      string                                             `json:"status"`
	Error       string                                             `json:"error,omitempty"`
	Convergence worker.ExactApplicationJobSpecificationConvergence `json:"convergence"`
}

func openStationReplayReport(path string) (*os.File, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("replay report path is required")
	}
	parent := filepath.Dir(filepath.Clean(path))
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("replay report parent directory is unavailable: %s", parent)
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

func newStationReplayReportEncoder(report *os.File) *json.Encoder {
	encoder := json.NewEncoder(report)
	encoder.SetEscapeHTML(false)
	return encoder
}

func writeStationReplayReport(encoder *json.Encoder, report *os.File, value any) error {
	if err := encoder.Encode(value); err != nil {
		return err
	}
	return report.Sync()
}

func stationReplayBase64(value []byte) string {
	return base64.StdEncoding.EncodeToString(value)
}

func newStationConvergenceReportRun(
	started time.Time,
	finished time.Time,
	convergence worker.ExactTypeScriptConvergence,
	convergenceErr error,
) stationConvergenceReportRun {
	convergence.WallDuration = finished.Sub(started)
	run := stationConvergenceReportRun{
		Type: "run", StartedAt: started, FinishedAt: finished,
		Status: "passed", Convergence: convergence,
		Evidence: make([]stationConvergenceIterationEvidence, 0, len(convergence.Iterations)),
	}
	if convergenceErr != nil {
		run.Status, run.Error = "failed", convergenceErr.Error()
	}
	for _, iteration := range convergence.Iterations {
		run.Evidence = append(run.Evidence, stationConvergenceIterationEvidence{
			Number: iteration.Number,
			GuidanceProviderResponseCaptureBase64: stationReplayBase64(
				iteration.GuidanceReplay.Generation.ProviderResponseCapture,
			),
			GuidanceProviderIdentityEvidence: iteration.GuidanceReplay.Generation.ProviderIdentityEvidence,
			ExecutorProviderResponseCaptureBase64: stationReplayBase64(
				iteration.ExecutionReplay.Generation.ProviderResponseCapture,
			),
			ExecutorProviderIdentityEvidence: iteration.ExecutionReplay.Generation.ProviderIdentityEvidence,
		})
	}
	return run
}

func newStationSpecificationConvergenceReportRun(
	started time.Time,
	finished time.Time,
	convergence worker.ExactApplicationJobSpecificationConvergence,
	convergenceErr error,
) stationSpecificationConvergenceReportRun {
	convergence.WallDuration = finished.Sub(started)
	run := stationSpecificationConvergenceReportRun{
		Type: "run", StartedAt: started, FinishedAt: finished,
		Status: "passed", Convergence: convergence,
	}
	if convergenceErr != nil {
		run.Status, run.Error = "failed", convergenceErr.Error()
	}
	return run
}

func printStationSpecificationConvergenceRun(run stationSpecificationConvergenceReportRun) {
	for _, call := range run.Convergence.Calls {
		generation := call.Replay.Generation
		fmt.Printf(
			"specification_convergence planner=%s call=%d kind=%s model=%s wall_ms=%d prompt_tokens=%d output_tokens=%d artifact=%s\n",
			run.Convergence.PlannerModel, call.Number,
			call.WorkKind, call.Model, call.Replay.WallDuration.Milliseconds(),
			generation.Usage.PromptEvalCount, generation.Usage.EvalCount, call.Replay.Artifact.Kind,
		)
	}
	fmt.Printf(
		"specification_convergence planner=%s status=%s terminal=%s calls=%d wall_ms=%d error=%s\n",
		run.Convergence.PlannerModel, run.Status,
		run.Convergence.Terminal, len(run.Convergence.Calls),
		run.Convergence.WallDuration.Milliseconds(), run.Error,
	)
}

func printStationConvergenceRun(run stationConvergenceReportRun) {
	for _, iteration := range run.Convergence.Iterations {
		fmt.Printf(
			"guided_convergence guidance_model=%s executor_model=%s iteration=%d guidance_wall_ms=%d guidance_prompt_tokens=%d guidance_output_tokens=%d instruction_bytes=%d executor_wall_ms=%d executor_prompt_tokens=%d executor_output_tokens=%d artifact_bytes=%d diagnostics_after=%s progress=%q guidance_artifact_error=%q executor_artifact_error=%q\n",
			run.Convergence.GuidanceModel, run.Convergence.ExecutorModel, iteration.Number,
			iteration.GuidanceReplay.WallDuration.Milliseconds(),
			iteration.GuidanceReplay.Generation.Usage.PromptEvalCount,
			iteration.GuidanceReplay.Generation.Usage.EvalCount, len(iteration.Instruction),
			iteration.ExecutionReplay.WallDuration.Milliseconds(),
			iteration.ExecutionReplay.Generation.Usage.PromptEvalCount,
			iteration.ExecutionReplay.Generation.Usage.EvalCount,
			len(iteration.ExecutionReplay.Artifact.Source),
			stationConvergenceDiagnosticSummary(iteration),
			stationConvergenceProgressSummary(iteration),
			iteration.GuidanceArtifactError, iteration.ExecutionArtifactError,
		)
	}
	fmt.Printf(
		"guided_convergence guidance_model=%s executor_model=%s status=%s terminal=%s iterations=%d wall_ms=%d error=%s\n",
		run.Convergence.GuidanceModel, run.Convergence.ExecutorModel,
		run.Status, run.Convergence.Terminal,
		len(run.Convergence.Iterations), run.Convergence.WallDuration.Milliseconds(), run.Error,
	)
}

func stationConvergenceProgressSummary(iteration worker.ExactTypeScriptConvergenceIteration) string {
	delta := iteration.DiagnosticDelta
	if delta == nil {
		return "not_scored"
	}
	return fmt.Sprintf(
		"before=%d after=%d resolved=%d retained=%d introduced=%d assessment=%s",
		delta.Before, delta.After, delta.Resolved, delta.Retained, delta.Introduced, delta.Assessment,
	)
}

func stationConvergenceDiagnosticSummary(iteration worker.ExactTypeScriptConvergenceIteration) string {
	if iteration.AfterDiagnostic == nil {
		return "not_compiled"
	}
	return strconv.Itoa(iteration.AfterDiagnostic.Count)
}

func printStationReplayRun(run stationReplayReportRun) {
	generation := run.Replay.Generation
	providerMS := generation.Usage.TotalDurationNanos / int64(time.Millisecond)
	promptTPS := stationReplayTokenRate(generation.Usage.PromptEvalCount, generation.Usage.PromptEvalDurationNanos)
	evalTPS := stationReplayTokenRate(generation.Usage.EvalCount, generation.Usage.EvalDurationNanos)
	if run.Status == "failed" {
		fmt.Printf("replay model=%s status=failed wall_ms=%d error=%s\n", run.Replay.Model,
			run.Replay.WallDuration.Milliseconds(), run.Error)
		return
	}
	fmt.Printf(
		"replay model=%s status=passed wall_ms=%d provider_ms=%d prompt_tokens=%d output_tokens=%d prompt_tps=%.2f eval_tps=%.2f thinking_bytes=%d final_bytes=%d artifact=%s artifact_bytes=%d changed=%t\n",
		run.Replay.Model, run.Replay.WallDuration.Milliseconds(), providerMS,
		generation.Usage.PromptEvalCount, generation.Usage.EvalCount, promptTPS, evalTPS,
		len(generation.Thinking), len(generation.Content), run.Replay.Artifact.Kind,
		len(run.Replay.Artifact.Source), run.Replay.Artifact.ChangedFromBase,
	)
}

func stationReplayTokenRate(tokens int, duration int64) float64 {
	if tokens <= 0 || duration <= 0 {
		return 0
	}
	return float64(tokens) / (float64(duration) / float64(time.Second))
}
