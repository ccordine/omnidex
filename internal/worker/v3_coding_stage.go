package worker

import (
	"fmt"
	"time"
)

const (
	maxDirectCodingStageCorrections         = 12
	maxDirectCodingStageRepeatedCorrections = 3
	directCodingStageTimeout                = 60 * time.Second
)

type directCodingStageDiagnostic struct {
	BlockID string
	Message string
	Output  string
}

func (s *directCodingSession) stageProgram(program *directCodingProgram) error {
	if program == nil {
		return fmt.Errorf("stage deterministic browser program: program is nil")
	}
	return s.stageTypeScriptProgram(program)
}
