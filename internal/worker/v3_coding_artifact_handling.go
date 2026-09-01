package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

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
		decision, err := runDirectCodingSemanticLeafCall(
			runtime, modelName, "artifact_handling", job, identities,
			func(raw string) (assemblyline.ArtifactHandlingDecision, error) {
				return assemblyline.DecodeArtifactHandlingDecision(input, raw)
			},
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

// sieveDirectCodingApplicationArtifactDirectives keeps only artifact state
// that has a current consumer. The active compiler consumes only exact
// preservation. Other classifications remain local intake results and cannot
// become a later veto merely because no current adapter consumes them.
func sieveDirectCodingApplicationArtifactDirectives(
	directives []assemblyline.ArtifactDirective,
) ([]assemblyline.ArtifactDirective, error) {
	retained := make([]assemblyline.ArtifactDirective, 0, len(directives))
	for index, directive := range directives {
		switch directive.Disposition {
		case assemblyline.ArtifactReference,
			assemblyline.ArtifactAbsenceCandidate:
			continue
		case assemblyline.ArtifactProtect,
			assemblyline.ArtifactRequire,
			assemblyline.ArtifactForbid:
			retained = append(retained, directive)
		default:
			return nil, fmt.Errorf(
				"application artifact directive %d has unsupported disposition %q",
				index, directive.Disposition,
			)
		}
	}
	return retained, nil
}
