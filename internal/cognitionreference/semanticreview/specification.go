package semanticreview

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func validateSpecification(specification ReviewSpecification) error {
	if !validIdentity(string(specification.ID)) ||
		!validIdentity(string(specification.ObjectiveID)) ||
		!validText(specification.Question, maxSpecificationText) {
		return fmt.Errorf("%w: identity or question is invalid", ErrInvalidSpecification)
	}
	if len(specification.Evidence) == 0 || len(specification.Evidence) > maxReviewEvidence {
		return fmt.Errorf("%w: evidence count is outside bounds", ErrInvalidSpecification)
	}
	evidenceIDs := make(map[cognitionreference.EvidenceID]struct{}, len(specification.Evidence))
	currentCount := 0
	previousEvidence := cognitionreference.EvidenceID("")
	for index, evidence := range specification.Evidence {
		if !validIdentity(string(evidence.ID)) || (index > 0 && evidence.ID <= previousEvidence) {
			return fmt.Errorf("%w: evidence identities must be unique and ascending", ErrInvalidSpecification)
		}
		previousEvidence = evidence.ID
		switch evidence.Kind {
		case EvidenceFixed:
			if !validText(evidence.Content, maxSpecificationText) {
				return fmt.Errorf("%w: fixed evidence %q is invalid", ErrInvalidSpecification, evidence.ID)
			}
		case EvidenceCurrentArtifact:
			currentCount++
			if evidence.Content != "" {
				return fmt.Errorf("%w: current artifact evidence must not carry caller content", ErrInvalidSpecification)
			}
		default:
			return fmt.Errorf("%w: evidence %q has unknown kind %q", ErrInvalidSpecification, evidence.ID, evidence.Kind)
		}
		evidenceIDs[evidence.ID] = struct{}{}
	}
	if currentCount != 1 {
		return fmt.Errorf("%w: exactly one current artifact evidence slot is required", ErrInvalidSpecification)
	}
	if len(specification.Candidates) < 2 || len(specification.Candidates) > maxReviewCandidates {
		return fmt.Errorf("%w: candidate count is outside bounds", ErrInvalidSpecification)
	}
	noneCount := 0
	usedEvidence := make(map[cognitionreference.EvidenceID]struct{}, len(evidenceIDs))
	seenFindings := make(map[FindingCode]struct{}, len(specification.Candidates))
	previousCandidate := cognitionreference.CandidateID("")
	for index, candidate := range specification.Candidates {
		if !validIdentity(string(candidate.CandidateID)) ||
			(index > 0 && candidate.CandidateID <= previousCandidate) ||
			!validText(candidate.Summary, 1024) || !validIdentity(string(candidate.FindingCode)) {
			return fmt.Errorf("%w: candidate %d is invalid or out of order", ErrInvalidSpecification, index)
		}
		previousCandidate = candidate.CandidateID
		switch candidate.Kind {
		case FindingSemanticIssue:
			if candidate.FindingCode == FindingCodeNone {
				return fmt.Errorf("%w: issue candidate uses the reserved none code", ErrInvalidSpecification)
			}
		case FindingNone:
			noneCount++
			if candidate.FindingCode != FindingCodeNone {
				return fmt.Errorf("%w: none candidate has a non-none code", ErrInvalidSpecification)
			}
		default:
			return fmt.Errorf("%w: candidate %q has unknown kind", ErrInvalidSpecification, candidate.CandidateID)
		}
		if _, duplicate := seenFindings[candidate.FindingCode]; duplicate {
			return fmt.Errorf("%w: finding code %q is duplicated", ErrInvalidSpecification, candidate.FindingCode)
		}
		seenFindings[candidate.FindingCode] = struct{}{}
		if len(candidate.EvidenceIDs) == 0 || len(candidate.EvidenceIDs) > len(evidenceIDs) ||
			!sort.SliceIsSorted(candidate.EvidenceIDs, func(i, j int) bool { return candidate.EvidenceIDs[i] < candidate.EvidenceIDs[j] }) {
			return fmt.Errorf("%w: candidate %q evidence is invalid", ErrInvalidSpecification, candidate.CandidateID)
		}
		previous := cognitionreference.EvidenceID("")
		for evidenceIndex, id := range candidate.EvidenceIDs {
			if _, exists := evidenceIDs[id]; !exists || (evidenceIndex > 0 && id <= previous) {
				return fmt.Errorf("%w: candidate %q cites invalid evidence", ErrInvalidSpecification, candidate.CandidateID)
			}
			previous = id
			usedEvidence[id] = struct{}{}
		}
	}
	if noneCount != 1 || len(usedEvidence) != len(evidenceIDs) {
		return fmt.Errorf("%w: exactly one none and no unused evidence are required", ErrInvalidSpecification)
	}
	return nil
}

func preflightSpecification(specification ReviewSpecification) error {
	if !validIdentity(string(specification.ID)) ||
		!validIdentity(string(specification.ObjectiveID)) ||
		!validText(specification.Question, maxSpecificationText) ||
		len(specification.Evidence) == 0 || len(specification.Evidence) > maxReviewEvidence ||
		len(specification.Candidates) < 2 || len(specification.Candidates) > maxReviewCandidates {
		return fmt.Errorf("%w: top-level authority exceeds bounds", ErrInvalidSpecification)
	}
	for _, evidence := range specification.Evidence {
		if !validIdentity(string(evidence.ID)) || len(evidence.Content) > maxSpecificationText ||
			(evidence.Kind != EvidenceFixed && evidence.Kind != EvidenceCurrentArtifact) {
			return fmt.Errorf("%w: evidence exceeds bounds", ErrInvalidSpecification)
		}
	}
	for _, candidate := range specification.Candidates {
		if !validIdentity(string(candidate.CandidateID)) ||
			!validIdentity(string(candidate.FindingCode)) || len(candidate.Summary) > 1024 ||
			len(candidate.EvidenceIDs) == 0 || len(candidate.EvidenceIDs) > maxReviewEvidence ||
			(candidate.Kind != FindingSemanticIssue && candidate.Kind != FindingNone) {
			return fmt.Errorf("%w: candidate exceeds bounds", ErrInvalidSpecification)
		}
		for _, id := range candidate.EvidenceIDs {
			if !validIdentity(string(id)) {
				return fmt.Errorf("%w: candidate evidence identity exceeds bounds", ErrInvalidSpecification)
			}
		}
	}
	return nil
}

func cloneSpecification(value ReviewSpecification) ReviewSpecification {
	value.Evidence = append([]EvidenceDefinition{}, value.Evidence...)
	value.Candidates = append([]FindingDefinition{}, value.Candidates...)
	for index := range value.Candidates {
		value.Candidates[index].EvidenceIDs = append(
			[]cognitionreference.EvidenceID{}, value.Candidates[index].EvidenceIDs...,
		)
	}
	return value
}

func specificationDigest(value ReviewSpecification) string {
	fields := []string{
		"semantic-review-specification.v1", string(value.ID), string(value.ObjectiveID),
		value.Question, fmt.Sprintf("evidence:%d", len(value.Evidence)),
	}
	for index, evidence := range value.Evidence {
		fields = append(fields, fmt.Sprintf("evidence:%d", index), string(evidence.ID), string(evidence.Kind), evidence.Content)
	}
	fields = append(fields, fmt.Sprintf("candidates:%d", len(value.Candidates)))
	for index, candidate := range value.Candidates {
		fields = append(fields, fmt.Sprintf("candidate:%d", index), string(candidate.CandidateID), string(candidate.FindingCode), string(candidate.Kind), candidate.Summary)
		fields = append(fields, fmt.Sprintf("evidence_ids:%d", len(candidate.EvidenceIDs)))
		for index, id := range candidate.EvidenceIDs {
			fields = append(fields, fmt.Sprintf("evidence_id:%d", index), string(id))
		}
	}
	return digestFields(fields...)
}
