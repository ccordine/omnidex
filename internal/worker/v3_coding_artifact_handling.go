package worker

import "github.com/gryph/omnidex/internal/assemblyline"

func classifyArtifactHandling(
	runtime typedWorkerRuntime,
	modelName string,
	authority string,
	identities []assemblyline.ArtifactIdentity,
) ([]assemblyline.ArtifactDirective, error) {
	artifacts := make([]assemblyline.ArtifactDirective, 0, len(identities))
	for _, identity := range identities {
		input := assemblyline.ArtifactHandlingInput{UserRequest: authority, Token: identity.Token}
		job, err := assemblyline.NewArtifactHandlingJob(input)
		if err != nil {
			return nil, err
		}
		decision, err := runDirectCodingSemanticCall[assemblyline.ArtifactHandlingDecision](
			runtime, modelName, "artifact_handling", job, identities,
			func(value assemblyline.ArtifactHandlingDecision) error { return value.Validate(identity.Token) },
		)
		if err != nil {
			return nil, err
		}
		directive := assemblyline.ArtifactReference
		switch decision.Handling {
		case assemblyline.ArtifactPreserveUnchanged:
			directive = assemblyline.ArtifactProtect
		case assemblyline.ArtifactMustExist:
			directive = assemblyline.ArtifactRequire
		case assemblyline.ArtifactMustBeAbsent:
			directive = assemblyline.ArtifactForbid
		case assemblyline.ArtifactPossibleAbsenceCandidate:
			directive = assemblyline.ArtifactAbsenceCandidate
		case assemblyline.ArtifactMentionedOnly:
			directive = assemblyline.ArtifactReference
		}
		artifacts = append(artifacts, assemblyline.ArtifactDirective{
			Token: identity.Token, Disposition: directive,
		})
	}
	return artifacts, nil
}
