package assemblyline

import "github.com/gryph/omnidex/internal/roleplay"

func portableRepositoryConversationResponseMaximum(job PortableJob) (int, bool, error) {
	switch job.Kind {
	case WorkRepositoryRequirementCoverage:
		return maximumStringBytes(
			RepositoryRequirementRemains, RepositoryNoUncoveredRequirement,
		), true, nil
	case WorkRepositoryRequirement:
		return maxRequirementQuoteBytes, true, nil
	case WorkRepositoryEvidenceRelevanceLeaf:
		maximum, err := repositoryEvidenceRelevanceMaximum(job)
		return maximum, true, err
	case WorkRepositoryChangeOwner:
		maximum, err := repositoryChangeOwnerMaximum(job)
		return maximum, true, err
	case WorkContextRelevanceSelection:
		maximum, err := contextRelevanceMaximum(job)
		return maximum, true, err
	case WorkContextMinification:
		return MaxContextMinifiedBytes, true, nil
	case WorkConversationObjectiveKind:
		return maximumStringBytes(
			ObjectiveKindAnswer, ObjectiveKindRepositoryRead,
			ObjectiveKindWorkspaceMutation, ObjectiveKindExternalAnswer,
			ObjectiveKindStory, ObjectiveKindDatabaseRead,
		), true, nil
	case WorkConversationResponse:
		var input ConversationResponseInput
		if err := decodePortablePayload(job.Payload, &input); err != nil {
			return 0, true, err
		}
		if input.RoleplayIdentity != nil {
			return roleplay.MaxNarrativeResponseBytes, true, nil
		}
		return maxConversationResponseTextBytes, true, nil
	case WorkRoleplayGroundedResponseText:
		return maxRoleplayGroundedResponseBytes, true, nil
	case WorkRoleplayGroundedResponseEvidenceRelation:
		return maximumStringBytes(
			RoleplayGroundedEvidenceSupportsParagraph,
			RoleplayGroundedEvidenceDoesNotSupport,
		), true, nil
	case WorkRoleplayCanonFactCoverage:
		return maximumStringBytes(
			RoleplayCanonFactRemains, RoleplayNoUncoveredCanonFact,
		), true, nil
	case WorkRoleplayCanonFact:
		return roleplay.MaxCanonEventBytes, true, nil
	case WorkRoleplayOngoingAction:
		return roleplay.MaxOngoingActionBytes, true, nil
	case WorkGroundedAnswerText:
		return maxGroundedAnswerTextBytes, true, nil
	case WorkGroundedAnswerEvidenceRelation:
		return maximumStringBytes(
			GroundedEvidenceSupportsAnswer, GroundedEvidenceDoesNotSupport,
		), true, nil
	default:
		return 0, false, nil
	}
}

func repositoryEvidenceRelevanceMaximum(job PortableJob) (int, error) {
	var input RepositoryEvidenceRelevanceLeafInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	candidates := []string{RepositoryEvidenceNoRelevantCandidate}
	for _, candidate := range input.Candidates {
		candidates = append(candidates, candidate.EvidenceID)
	}
	return maximumAcceptedCandidateBytes(
		"repository evidence relevance", candidates,
		func(candidate string) error {
			_, err := DecodeRepositoryEvidenceRelevanceLeaf(input, candidate)
			return err
		},
	)
}

func repositoryChangeOwnerMaximum(job PortableJob) (int, error) {
	var input RepositoryChangeOwnerInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	candidates := []string{RepositoryChangeOwnerNone}
	for candidate := range eligibleRepositoryChangeOwnerIDs(input.Authority) {
		candidates = append(candidates, candidate)
	}
	return maximumAcceptedCandidateBytes(
		"repository change owner", candidates,
		func(candidate string) error {
			_, err := DecodeRepositoryChangeOwnerLeaf(input, candidate)
			return err
		},
	)
}

func contextRelevanceMaximum(job PortableJob) (int, error) {
	var input ContextRelevanceSelectionInput
	if err := decodePortablePayload(job.Payload, &input); err != nil {
		return 0, err
	}
	candidates := []string{ContextRelevanceNoCandidate}
	for _, candidate := range input.Authority.CandidateAuthorities {
		candidates = append(candidates, candidate.CandidateID)
	}
	return maximumAcceptedCandidateBytes(
		"context relevance selection", candidates,
		func(candidate string) error {
			_, err := DecodeContextRelevanceSelectionDecision(input, candidate)
			return err
		},
	)
}
