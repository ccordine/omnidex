package assemblyline

import "fmt"

func portableCodingResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkArtifactHandling:
		return maximumStringBytes(
			ArtifactPreserveUnchanged, ArtifactMustExist, ArtifactMustBeAbsent,
			ArtifactPossibleAbsenceCandidate, ArtifactMentionedOnly,
		), true, nil
	case WorkRepositoryArtifactAbsence:
		return maximumStringBytes(
			RepositoryArtifactMustBeAbsent, RepositoryArtifactAbsenceNotExplicit,
		), true, nil
	case WorkPlainTextArtifactCreation:
		return maximumStringBytes(
			OneNewCompletePlainTextArtifactRequired, PlainTextArtifactCreationNotExplicit,
		), true, nil
	case WorkDeclarationArtifactBoundary:
		return maximumStringBytes(
			DeclarationBoundaryIndependentArtifact,
			DeclarationBoundaryExistingArtifact, DeclarationBoundaryNone,
		), true, nil
	case WorkArtifactCandidateSelection:
		var input ArtifactCandidateSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		maximum := len(ArtifactCandidateSelectionNone)
		for _, candidate := range input.Candidates {
			maximum = max(maximum, len(candidate.CandidateID))
		}
		return maximum, true, nil
	case WorkCapabilityRelation:
		return maximumStringBytes(
			CapabilityIndependent, CapabilityLeftReadsRight,
			CapabilityRightReadsLeft,
		), true, nil
	case WorkSkillSelection:
		var input SkillSelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		maximum := len(SkillSelectionNone)
		for _, candidate := range input.Candidates {
			maximum = max(maximum, len(candidate.Token))
		}
		return maximum, true, nil
	case WorkRuntimeCapabilitySelection:
		var input RuntimeCapabilitySelectionInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		maximum := len(RuntimeCapabilitySelectionNone)
		for _, candidate := range input.Candidates {
			maximum = max(maximum, len(candidate.CandidateID))
		}
		return maximum, true, nil
	case WorkTypeScriptRepairGuidance:
		return maxTypeScriptRepairGuidanceBytes, true, nil
	case WorkFragmentGeneration:
		maximum, err := fragmentGenerationResponseMaximum(job)
		return maximum, true, err
	case WorkFragmentGenerationReplacement:
		maximum, err := fragmentGenerationReplacementResponseMaximum(job)
		return maximum, true, err
	case WorkFragmentModification:
		return MaxPortableRawCandidateBytes, true, nil
	case WorkFragmentCorrection:
		maximum, err := fragmentCorrectionResponseMaximum(job)
		return maximum, true, err
	default:
		return 0, false, nil
	}
}

func fragmentGenerationReplacementResponseMaximum(job PortableJob) (int, error) {
	var input FragmentGenerationReplacementInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	origin, err := NewFragmentGenerationJob(input.Original)
	if err != nil {
		return 0, err
	}
	return fragmentGenerationResponseMaximum(origin)
}

func fragmentGenerationResponseMaximum(job PortableJob) (int, error) {
	var input FragmentGenerationInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	switch input.Language {
	case "go", "typescript":
		return MaxPortableRawCandidateBytes, nil
	case TextFragmentLanguage, "javascript", "java", "rust", "php":
		return MaxPortableSemanticCandidateBytes, nil
	default:
		return 0, fmt.Errorf(
			"fragment generation language %q has no response maximum", input.Language,
		)
	}
}

func fragmentCorrectionResponseMaximum(job PortableJob) (int, error) {
	var input FragmentCorrectionInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	if job.SourceProjection != "" {
		if job.SourceProjection == "go" {
			return MaxPortableRawCandidateBytes, nil
		}
		if _, err := boundedSourceLanguageByID(job.SourceProjection); err != nil {
			return 0, err
		}
		return MaxPortableSemanticCandidateBytes, nil
	}
	if input.RepairRegion != nil &&
		input.RepairRegion.Kind == TypeScriptRepairRegionSyntaxWindow {
		return maxTypeScriptRepairRegionBytes, nil
	}
	return MaxPortableRawCandidateBytes, nil
}
