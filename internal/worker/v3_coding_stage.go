package worker

import (
	"time"
)

const directCodingStageTimeout = 60 * time.Second

type directCodingStageDiagnostic struct {
	BlockID                string
	Message                string
	Output                 string
	ModelFeedback          string
	VerificationStage      string
	FailureClass           directCodingStageFailureClass
	DeclarationLine        int
	DeclarationColumn      int
	CompilerIssue          bool
	DocumentPath           string
	DocumentLine           int
	DocumentColumn         int
	DocumentBlockStartLine int
	DocumentBlockEndLine   int
}
