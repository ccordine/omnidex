package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

type semanticCandidateExhaustedError struct {
	Subject  string
	Attempts int
	Err      error
}

func (err *semanticCandidateExhaustedError) Error() string {
	return fmt.Sprintf(
		"%s candidate failed after %d bounded attempts: %v",
		err.Subject, err.Attempts, err.Err,
	)
}

func (err *semanticCandidateExhaustedError) Unwrap() error {
	return err.Err
}

func validateDirectCodingSemanticPrompt(
	prompt string,
	identities []assemblyline.ArtifactIdentity,
	provenance assemblyline.ArtifactIdentityProvenance,
) error {
	for _, identity := range identities {
		value := strings.TrimSpace(identity.Value)
		if value != "" && strings.Contains(prompt, value) {
			return fmt.Errorf(
				"coding semantic prompt exposes source identity behind %s",
				identity.Token,
			)
		}
	}
	if matches := modelcontext.ProvenArtifactIdentities(prompt, provenance); len(matches) > 0 {
		return fmt.Errorf(
			"coding semantic prompt exposes known artifact identity %q",
			prompt[matches[0].Start:matches[0].End],
		)
	}
	return nil
}

func emitDirectCodingSemanticRejection(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	attempt int,
	err error,
) {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerRejected, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
}

func failDirectCodingSemanticCall(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	attempt int,
	err error,
) error {
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerFailed, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
	return fmt.Errorf("coding semantic %s failed: %w", subject, err)
}
