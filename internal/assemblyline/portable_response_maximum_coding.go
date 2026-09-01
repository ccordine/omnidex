package assemblyline

import "fmt"

func portableCodingResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkArtifactHandling:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(artifactHandlingOpaqueChoices)
		return maximum, true, err
	case WorkCapabilityRelation:
		maximum, err := opaqueModelChoiceBuilderResponseMaximum(capabilityRelationOpaqueChoices)
		return maximum, true, err
	case WorkFragmentGeneration:
		maximum, err := fragmentGenerationResponseMaximum(job)
		return maximum, true, err
	default:
		return 0, false, nil
	}
}

func fragmentGenerationResponseMaximum(job PortableJob) (int, error) {
	var input FragmentGenerationInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	switch input.Language {
	case "go", "typescript", "javascript", "java", "rust":
		return MaxPortableRawCandidateBytes, nil
	case TextFragmentLanguage:
		return MaxPortableSemanticCandidateBytes, nil
	default:
		return 0, fmt.Errorf(
			"fragment generation language %q has no response maximum", input.Language,
		)
	}
}
