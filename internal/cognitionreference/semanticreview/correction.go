package semanticreview

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func deriveCorrectionObjective(
	root Objective,
	review ReviewObjective,
	finding ReviewFinding,
	artifact Artifact,
	rule CorrectionRule,
) (CorrectionObjective, error) {
	if finding.Kind != FindingSemanticIssue || finding.FindingCode == FindingCodeNone ||
		finding.RootObjectiveID != root.ID || finding.ReviewObjectiveID != review.ID ||
		finding.GapID != review.GapID || finding.ArtifactID != artifact.ID ||
		finding.ArtifactSHA256 != artifact.SHA256 || rule.FindingCode != finding.FindingCode {
		return CorrectionObjective{}, fmt.Errorf("%w: finding, review, artifact, and rule are not exactly bound", ErrCorrection)
	}
	objective := CorrectionObjective{
		RootObjectiveID: root.ID, ParentID: review.ID,
		DependsOn: []cognitionreference.ObjectiveID{review.ID}, Round: review.Round,
		Finding: cloneFinding(finding), InputArtifactID: artifact.ID, InputSHA256: artifact.SHA256,
		ObjectiveKind: rule.ObjectiveKind,
		Acceptance:    append([]CorrectionAcceptancePredicate{}, rule.Acceptance...),
		Status:        ObjectivePending,
	}
	objective.ID = correctionIdentity(objective)
	return cloneCorrectionObjective(objective), nil
}

func correctionIdentity(objective CorrectionObjective) cognitionreference.ObjectiveID {
	return cognitionreference.ObjectiveID("C" + digestFields(
		string(objective.RootObjectiveID), string(objective.ParentID),
		string(objective.Finding.ID), string(objective.InputArtifactID),
		objective.InputSHA256, string(objective.ObjectiveKind),
	))
}

func cloneCorrectionObjective(value CorrectionObjective) CorrectionObjective {
	value.DependsOn = append([]cognitionreference.ObjectiveID{}, value.DependsOn...)
	value.Finding = cloneFinding(value.Finding)
	value.Acceptance = append([]CorrectionAcceptancePredicate{}, value.Acceptance...)
	return value
}
