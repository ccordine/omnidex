package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func validatePortableSemanticPrompt(
	prompt string,
	identities []assemblyline.ArtifactIdentity,
	provenance assemblyline.ArtifactIdentityProvenance,
) error {
	for _, identity := range identities {
		value := strings.TrimSpace(identity.Value)
		if value != "" && strings.Contains(prompt, value) {
			return fmt.Errorf(
				"portable semantic prompt exposes source identity behind %s",
				identity.Token,
			)
		}
	}
	if matches := modelcontext.ProvenArtifactIdentities(prompt, provenance); len(matches) > 0 {
		return fmt.Errorf(
			"portable semantic prompt exposes known artifact identity %q",
			prompt[matches[0].Start:matches[0].End],
		)
	}
	return nil
}

func validatePortableSemanticCandidatePathBoundary(
	kind assemblyline.WorkKind,
	candidate string,
	provenance assemblyline.ArtifactIdentityProvenance,
) error {
	switch kind {
	case assemblyline.WorkContextMinification:
		return assemblyline.ValidateMinifiedContextText(candidate, provenance)
	case assemblyline.WorkFragmentGeneration:
		return fmt.Errorf(
			"work kind %q cannot use the raw semantic-leaf candidate boundary",
			kind,
		)
	default:
		return assemblyline.ValidatePathFreeModelContextWithProvenance(
			"portable semantic result", provenance, candidate,
		)
	}
}

func emitPortableSemanticRejection(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	attempt int,
	err error,
) {
	if typedWorkerFailureIsStop(runtime, err) {
		return
	}
	emitTypedWorker(runtime, typedWorkerEvent{
		State: typedWorkerRejected, Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
		Detail: trimForBudget(err.Error(), 1200),
	})
}

func failPortableSemanticCall(
	runtime typedWorkerRuntime,
	modelName string,
	subject string,
	attempt int,
	err error,
) error {
	emitTypedWorkerFailure(runtime, typedWorkerEvent{
		Kind: typedWorkerSemantic, Subject: subject,
		Model: modelName, Attempt: attempt, MaxAttempts: runtime.MaxAttempts,
	}, err)
	return fmt.Errorf("portable semantic %s failed: %w", subject, err)
}
