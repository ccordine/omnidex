package worker

import (
	"time"
)

const directCodingStageTimeout = 60 * time.Second

type directCodingStageDiagnostic struct {
	BlockID           string
	Message           string
	Output            string
	VerificationStage string
	FailureClass      directCodingStageFailureClass
}
