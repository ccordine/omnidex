package worker

import (
	"context"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type ExactTypeScriptConvergenceTerminal string

const (
	ExactTypeScriptConvergenceCompiled ExactTypeScriptConvergenceTerminal = "compiled"
	ExactTypeScriptConvergenceNoOp     ExactTypeScriptConvergenceTerminal = "no_op"
	ExactTypeScriptConvergenceCycle    ExactTypeScriptConvergenceTerminal = "cycle"
	ExactTypeScriptConvergenceFailed   ExactTypeScriptConvergenceTerminal = "failed"
)

type ExactTypeScriptReplayDiagnostic struct {
	ModelFeedback        string   `json:"model_feedback"`
	ModelFeedbackSHA256  string   `json:"model_feedback_sha256"`
	CompilerOutputSHA256 string   `json:"compiler_output_sha256"`
	CompilerDiagnostics  []string `json:"compiler_diagnostics"`
	Count                int      `json:"count"`
}

type ExactTypeScriptConvergenceIteration struct {
	Number          int                              `json:"number"`
	Replay          ExactStationReplay               `json:"replay"`
	ArtifactError   string                           `json:"artifact_error,omitempty"`
	AfterDiagnostic *ExactTypeScriptReplayDiagnostic `json:"after_diagnostic,omitempty"`
}

type ExactStationReplayArtifactError struct {
	Cause error
}

func (failure *ExactStationReplayArtifactError) Error() string {
	if failure == nil || failure.Cause == nil {
		return "station replay artifact is invalid"
	}
	return failure.Cause.Error()
}

func (failure *ExactStationReplayArtifactError) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

type ExactTypeScriptConvergence struct {
	SourceOpeningID    int64                                 `json:"source_opening_id"`
	SourceGapOpeningID int64                                 `json:"source_gap_opening_id"`
	Model              string                                `json:"model"`
	Baseline           ExactTypeScriptReplayDiagnostic       `json:"baseline"`
	Iterations         []ExactTypeScriptConvergenceIteration `json:"iterations"`
	Terminal           ExactTypeScriptConvergenceTerminal    `json:"terminal"`
	FinalSource        string                                `json:"final_source"`
	FinalSourceSHA256  string                                `json:"final_source_sha256"`
	WallDuration       time.Duration                         `json:"wall_duration"`
}

type exactTypeScriptConvergenceRuntime struct {
	verify func(context.Context, string) (*ExactTypeScriptReplayDiagnostic, error)
	replay func(context.Context, assemblyline.PortableJob, int) (ExactStationReplay, error)
}
