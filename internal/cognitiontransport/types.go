package cognitiontransport

import (
	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

const (
	ProtocolVersionV1 = "omnidex.cognition-environment-http.v1"
	startPath         = "/v1/cognition/start"
	applyPath         = "/v1/cognition/apply"
	evaluatePath      = "/v1/cognition/evaluate"
	maxRequestBytes   = 1024 * 1024
)

type startRequest struct {
	Protocol string                `json:"protocol"`
	Scenario cognition.ScenarioRef `json:"scenario"`
}

type applyRequest struct {
	Protocol string                     `json:"protocol"`
	Episode  cognition.EpisodeRef       `json:"episode"`
	Expected cognition.WorldRevision    `json:"expected_revision"`
	Action   cognition.RegisteredAction `json:"action"`
}

type evaluateRequest struct {
	Protocol string                             `json:"protocol"`
	Request  cognitionruntime.CompletionRequest `json:"request"`
}

type wireResponse struct {
	Protocol   string                      `json:"protocol"`
	Transition *cognition.Transition       `json:"transition,omitempty"`
	Failure    *cognition.ActionFailure    `json:"failure,omitempty"`
	Completion *cognition.CompletionResult `json:"completion,omitempty"`
	Error      *wireError                  `json:"error,omitempty"`
}

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
