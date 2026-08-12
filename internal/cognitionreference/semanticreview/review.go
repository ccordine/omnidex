package semanticreview

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func buildReview(
	root Objective,
	specification ReviewSpecification,
	artifact Artifact,
	parent cognitionreference.ObjectiveID,
	round int,
) (ReviewObjective, cognitionreference.SemanticGap, error) {
	if err := validateArtifact(artifact); err != nil {
		return ReviewObjective{}, cognitionreference.SemanticGap{}, err
	}
	reviewID := reviewIdentity(root.ID, parent, artifact.ID, artifact.SHA256, round, specification)
	gapID := reviewGapIdentity(reviewID, artifact.ID, artifact.SHA256)
	review := ReviewObjective{
		ID: reviewID, RootObjectiveID: root.ID, ParentID: parent, Round: round,
		ArtifactID: artifact.ID, ArtifactSHA256: artifact.SHA256, GapID: gapID,
		Acceptance: []ReviewAcceptancePredicate{AcceptanceReviewFindingResolved},
		Status:     ObjectivePending,
	}
	if parent != root.ID {
		review.DependsOn = []cognitionreference.ObjectiveID{parent}
	} else {
		review.DependsOn = []cognitionreference.ObjectiveID{}
	}
	gap := cognitionreference.SemanticGap{
		ID: gapID, Kind: cognitionreference.GapCandidateSelection,
		ObjectiveID: reviewID, Question: specification.Question,
		Evidence:   make([]cognitionreference.SemanticEvidence, len(specification.Evidence)),
		Candidates: make([]cognitionreference.SemanticCandidate, len(specification.Candidates)),
	}
	for index, evidence := range specification.Evidence {
		content := evidence.Content
		if evidence.Kind == EvidenceCurrentArtifact {
			content = string(artifact.Content)
		}
		gap.Evidence[index] = cognitionreference.SemanticEvidence{ID: evidence.ID, Content: content}
	}
	for index, candidate := range specification.Candidates {
		gap.Candidates[index] = cognitionreference.SemanticCandidate{
			ID: candidate.CandidateID, Summary: candidate.Summary,
			EvidenceIDs: append([]cognitionreference.EvidenceID{}, candidate.EvidenceIDs...),
		}
	}
	if err := gap.Validate(); err != nil {
		return ReviewObjective{}, cognitionreference.SemanticGap{}, fmt.Errorf("%w: rendered gap: %v", ErrInvalidSpecification, err)
	}
	return cloneReviewObjective(review), gap.Clone(), nil
}

func materializeFinding(
	root Objective,
	specification ReviewSpecification,
	review ReviewObjective,
	artifact Artifact,
	selected cognitionreference.CandidateID,
) (ReviewFinding, error) {
	var definition *FindingDefinition
	for index := range specification.Candidates {
		if specification.Candidates[index].CandidateID == selected {
			candidate := specification.Candidates[index]
			definition = &candidate
			break
		}
	}
	if definition == nil {
		return ReviewFinding{}, fmt.Errorf("%w: selected candidate %q is absent", ErrInvalidSpecification, selected)
	}
	finding := ReviewFinding{
		RootObjectiveID: root.ID, ReviewObjectiveID: review.ID, GapID: review.GapID,
		ArtifactID: artifact.ID, ArtifactSHA256: artifact.SHA256,
		CandidateID: definition.CandidateID, Kind: definition.Kind,
		FindingCode: definition.FindingCode,
		EvidenceIDs: append([]cognitionreference.EvidenceID{}, definition.EvidenceIDs...),
	}
	finding.ID = findingIdentity(finding)
	return cloneFinding(finding), nil
}

func reviewIdentity(
	root, parent cognitionreference.ObjectiveID,
	artifact ArtifactID,
	artifactSHA string,
	round int,
	specification ReviewSpecification,
) cognitionreference.ObjectiveID {
	return cognitionreference.ObjectiveID("R" + digestFields(
		string(root), string(parent), string(artifact), artifactSHA,
		fmt.Sprintf("%d", round), specificationDigest(specification),
	))
}

func reviewGapIdentity(
	review cognitionreference.ObjectiveID,
	artifact ArtifactID,
	artifactSHA string,
) cognitionreference.GapID {
	return cognitionreference.GapID("G" + digestFields(string(review), string(artifact), artifactSHA))
}

func findingIdentity(finding ReviewFinding) FindingID {
	return FindingID("F" + digestFields(
		string(finding.RootObjectiveID), string(finding.ReviewObjectiveID), string(finding.GapID),
		string(finding.ArtifactID), finding.ArtifactSHA256, string(finding.CandidateID),
		string(finding.Kind), string(finding.FindingCode),
	))
}

func cloneReviewObjective(value ReviewObjective) ReviewObjective {
	value.DependsOn = append([]cognitionreference.ObjectiveID{}, value.DependsOn...)
	value.Acceptance = append([]ReviewAcceptancePredicate{}, value.Acceptance...)
	return value
}

func cloneFinding(value ReviewFinding) ReviewFinding {
	value.EvidenceIDs = append([]cognitionreference.EvidenceID{}, value.EvidenceIDs...)
	return value
}
