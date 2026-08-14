package worker

import (
	"time"
)

const (
	maxDirectCodingStageCorrections         = 3
	maxDirectCodingStageRepeatedCorrections = 3
	directCodingStageTimeout                = 60 * time.Second
)

type directCodingStageDiagnostic struct {
	BlockID      string
	Message      string
	Output       string
	FailureClass directCodingStageFailureClass
}
