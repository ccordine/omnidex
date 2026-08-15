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
	Stage                ExactTypeScriptVerificationStage             `json:"stage"`
	ModelFeedback        string                                       `json:"model_feedback"`
	ModelFeedbackSHA256  string                                       `json:"model_feedback_sha256"`
	CompilerOutputSHA256 string                                       `json:"compiler_output_sha256"`
	CompilerDiagnostics  []string                                     `json:"compiler_diagnostics"`
	Count                int                                          `json:"count"`
	RepairRegion         *assemblyline.TypeScriptFragmentRepairRegion `json:"repair_region,omitempty"`
}

type ExactTypeScriptVerificationStage string

const (
	ExactTypeScriptVerificationSyntax    ExactTypeScriptVerificationStage = "syntax"
	ExactTypeScriptVerificationTypecheck ExactTypeScriptVerificationStage = "typecheck"
	ExactTypeScriptVerificationCompiled  ExactTypeScriptVerificationStage = "compiled"
)

type ExactTypeScriptConvergenceAssessment string

const (
	ExactTypeScriptConvergenceCompiledAssessment ExactTypeScriptConvergenceAssessment = "compiled"
	ExactTypeScriptConvergenceProgress           ExactTypeScriptConvergenceAssessment = "progress"
	ExactTypeScriptConvergenceMixed              ExactTypeScriptConvergenceAssessment = "mixed"
	ExactTypeScriptConvergenceUnchanged          ExactTypeScriptConvergenceAssessment = "unchanged"
	ExactTypeScriptConvergenceRegression         ExactTypeScriptConvergenceAssessment = "regression"
)

type ExactTypeScriptDiagnosticDelta struct {
	BeforeStage ExactTypeScriptVerificationStage     `json:"before_stage"`
	AfterStage  ExactTypeScriptVerificationStage     `json:"after_stage"`
	Before      int                                  `json:"before"`
	After       int                                  `json:"after"`
	Resolved    int                                  `json:"resolved"`
	Retained    int                                  `json:"retained"`
	Introduced  int                                  `json:"introduced"`
	Assessment  ExactTypeScriptConvergenceAssessment `json:"assessment"`
}

type ExactTypeScriptConvergenceIteration struct {
	Number          int                              `json:"number"`
	Replay          ExactStationReplay               `json:"replay"`
	ArtifactError   string                           `json:"artifact_error,omitempty"`
	AfterDiagnostic *ExactTypeScriptReplayDiagnostic `json:"after_diagnostic,omitempty"`
	DiagnosticDelta *ExactTypeScriptDiagnosticDelta  `json:"diagnostic_delta,omitempty"`
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
