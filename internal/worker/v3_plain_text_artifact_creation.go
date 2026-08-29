package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func classifyPlainTextArtifactCreation(
	runtime typedWorkerRuntime,
	modelName string,
	input assemblyline.PlainTextArtifactCreationInput,
	identities []assemblyline.ArtifactIdentity,
) (assemblyline.PlainTextArtifactCreationDecision, error) {
	job, err := assemblyline.NewPlainTextArtifactCreationJob(input)
	if err != nil {
		return assemblyline.PlainTextArtifactCreationDecision{}, err
	}
	return runDirectCodingSemanticLeafCall(
		runtime, modelName, "plain_text_artifact_creation", job, identities,
		func(raw string) (assemblyline.PlainTextArtifactCreationDecision, error) {
			return assemblyline.DecodePlainTextArtifactCreationDecision(input, raw)
		},
		func(value assemblyline.PlainTextArtifactCreationDecision) error {
			if err := value.ValidateFor(input); err != nil {
				return fmt.Errorf("validate plain-text artifact creation: %w", err)
			}
			return nil
		},
	)
}
