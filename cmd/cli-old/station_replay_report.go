package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/worker"
)

const stationReplayReportSchema = "omnidex.station-replay-report.v2"

type stationReplayReportHeader struct {
	Type                        string                   `json:"type"`
	Schema                      string                   `json:"schema"`
	CreatedAt                   time.Time                `json:"created_at"`
	SourceCallOpening           queue.StationCallOpening `json:"source_call_opening"`
	SourceCallWireRequestBase64 string                   `json:"source_call_wire_request_base64"`
	SourceGapOpening            queue.StationGapOpening  `json:"source_gap_opening"`
	Models                      []string                 `json:"models"`
	Timeout                     string                   `json:"timeout"`
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
		"replay model=%s status=passed wall_ms=%d provider_ms=%d prompt_tokens=%d output_tokens=%d prompt_tps=%.2f eval_tps=%.2f final_bytes=%d artifact=%s artifact_bytes=%d changed=%t\n",
		run.Replay.Model, run.Replay.WallDuration.Milliseconds(), providerMS,
		generation.Usage.PromptEvalCount, generation.Usage.EvalCount, promptTPS, evalTPS,
		len(generation.Content), run.Replay.Artifact.Kind,
		len(run.Replay.Artifact.Source), run.Replay.Artifact.ChangedFromBase,
	)
}

func stationReplayTokenRate(tokens int, duration int64) float64 {
	if tokens <= 0 || duration <= 0 {
		return 0
	}
	return float64(tokens) / (float64(duration) / float64(time.Second))
}
