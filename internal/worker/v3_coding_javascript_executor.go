package worker

import (
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const directCodingJavaScriptStageTimeout = 2 * time.Minute

func newDirectCodingJavaScriptProjectStageExecutor(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectStageExecutor, error) {
	return newDirectCodingLanguageProjectStageExecutor(session, directCodingLanguageStageConfig{
		Language: "javascript", AdapterID: "javascript",
		Timeout:            directCodingJavaScriptStageTimeout,
		ValidateFragment:   validateDirectCodingJavaScriptFragment,
		ValidateAcceptance: validateDirectCodingJavaScriptAcceptance,
		TaskCommands: func(
			assemblyline.ApplicationTaskContext,
			directCodingProgram,
		) ([]testCommand, error) {
			return []testCommand{{Family: "node", Name: "node", Args: javaScriptNodeTestArgs(), Purpose: verificationTest}}, nil
		},
		FinalCommands: func(directCodingProgram) ([]testCommand, error) {
			return javaScriptCommandLineVerificationCommandSet(), nil
		},
	})
}
