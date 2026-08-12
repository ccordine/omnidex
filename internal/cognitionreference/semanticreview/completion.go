package semanticreview

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionreference"
)

func completeResult(
	result *Result,
	specification ReviewSpecification,
	rules CorrectionRuleRegistry,
) error {
	if result == nil || result.Objective.Status != ObjectivePending ||
		!reflect.DeepEqual(result.Objective.Acceptance, exactRootAcceptance) ||
		specification.ObjectiveID != result.Objective.ID {
		return fmt.Errorf("%w: root objective authority is invalid", ErrCompletion)
	}
	if err := validateArtifact(result.InitialArtifact); err != nil {
		return err
	}
	if err := validateArtifact(result.CurrentArtifact); err != nil {
		return err
	}
	if len(result.Reviews) == 0 || len(result.Findings) != len(result.Reviews) ||
		len(result.Corrections) != len(result.Reviews)-1 ||
		len(result.VerificationReceipts) != len(result.Reviews) {
		return fmt.Errorf("%w: recursive topology is incomplete", ErrCompletion)
	}
	if err := validateInitialReceipt(result); err != nil {
		return err
	}
	parent := result.Objective.ID
	artifactID := result.InitialArtifact.ID
	artifactSHA := result.InitialArtifact.SHA256
	for index := range result.Reviews {
		review := result.Reviews[index]
		finding := result.Findings[index]
		receipt := result.VerificationReceipts[index]
		if err := validateReviewNode(
			result, specification, review, finding, receipt,
			parent, artifactID, artifactSHA, index,
		); err != nil {
			return err
		}
		if index == len(result.Reviews)-1 {
			break
		}
		correction := result.Corrections[index]
		if err := validateCorrectionNode(
			result, rules, review, finding, correction, index,
		); err != nil {
			return err
		}
		nextReceipt := result.VerificationReceipts[index+1]
		if err := validateCorrectionReceipt(
			result.Objective, correction, nextReceipt, index,
		); err != nil {
			return err
		}
		parent = correction.ID
		artifactID = nextReceipt.ArtifactID
		artifactSHA = nextReceipt.ArtifactSHA256
	}
	finalFinding := result.Findings[len(result.Findings)-1]
	finalReview := result.Reviews[len(result.Reviews)-1]
	finalReceipt := result.VerificationReceipts[len(result.VerificationReceipts)-1]
	if finalFinding.Kind != FindingNone || finalFinding.FindingCode != FindingCodeNone ||
		finalFinding.ArtifactID != result.CurrentArtifact.ID ||
		finalReview.ArtifactID != result.CurrentArtifact.ID ||
		finalReceipt.ArtifactID != result.CurrentArtifact.ID ||
		finalReceipt.ArtifactSHA256 != result.CurrentArtifact.SHA256 ||
		finalReceipt.ArtifactRevision != result.CurrentArtifact.Revision {
		return fmt.Errorf("%w: final none and current verified artifact do not match", ErrCompletion)
	}
	if result.CurrentArtifact.Revision != uint32(len(result.Corrections)+1) {
		return fmt.Errorf("%w: artifact revision does not match correction count", ErrCompletion)
	}
	if len(result.Corrections) != 0 {
		latest := result.Corrections[len(result.Corrections)-1]
		if result.CurrentArtifact.ParentID != latest.InputArtifactID ||
			result.CurrentArtifact.ID != latest.OutputArtifactID {
			return fmt.Errorf("%w: current artifact lineage is not bound to final correction", ErrCompletion)
		}
	}
	result.Objective.Status = ObjectiveComplete
	result.Complete = true
	return nil
}

func validateInitialReceipt(result *Result) error {
	receipt := result.VerificationReceipts[0]
	if receipt.ID != verificationReceiptIdentity(receipt) ||
		receipt.Kind != VerificationCurrentArtifact ||
		receipt.RootObjectiveID != result.Objective.ID ||
		receipt.ArtifactID != result.InitialArtifact.ID ||
		receipt.ArtifactSHA256 != result.InitialArtifact.SHA256 ||
		receipt.ArtifactRevision != result.InitialArtifact.Revision ||
		receipt.CorrectionObjectiveID != "" || len(receipt.CorrectionAcceptance) != 0 ||
		!reflect.DeepEqual(receipt.ArtifactAcceptance, exactArtifactAcceptance) {
		return fmt.Errorf("%w: initial deterministic receipt is invalid", ErrCompletion)
	}
	return nil
}

func validateReviewNode(
	result *Result,
	specification ReviewSpecification,
	review ReviewObjective,
	finding ReviewFinding,
	receipt VerificationReceipt,
	parent cognitionreference.ObjectiveID,
	artifactID ArtifactID,
	artifactSHA string,
	index int,
) error {
	wantDependencies := []cognitionreference.ObjectiveID{}
	if index > 0 {
		wantDependencies = []cognitionreference.ObjectiveID{parent}
	}
	wantReviewID := reviewIdentity(
		result.Objective.ID, parent, artifactID, artifactSHA, index+1, specification,
	)
	if review.ID != wantReviewID || review.RootObjectiveID != result.Objective.ID ||
		review.ParentID != parent || !reflect.DeepEqual(review.DependsOn, wantDependencies) ||
		review.Round != index+1 || review.ArtifactID != artifactID ||
		review.ArtifactSHA256 != artifactSHA ||
		review.GapID != reviewGapIdentity(review.ID, artifactID, artifactSHA) ||
		review.Status != ObjectiveComplete ||
		!reflect.DeepEqual(review.Acceptance, []ReviewAcceptancePredicate{AcceptanceReviewFindingResolved}) {
		return fmt.Errorf("%w: review %d authority is invalid", ErrCompletion, index+1)
	}
	definition, found := findingDefinition(specification, finding.CandidateID)
	if !found || finding.ID != findingIdentity(finding) ||
		finding.RootObjectiveID != result.Objective.ID || finding.ReviewObjectiveID != review.ID ||
		finding.GapID != review.GapID || finding.ArtifactID != artifactID ||
		finding.ArtifactSHA256 != artifactSHA || finding.Kind != definition.Kind ||
		finding.FindingCode != definition.FindingCode ||
		!reflect.DeepEqual(finding.EvidenceIDs, definition.EvidenceIDs) ||
		receipt.ArtifactID != artifactID || receipt.ArtifactSHA256 != artifactSHA {
		return fmt.Errorf("%w: review %d finding or receipt is invalid", ErrCompletion, index+1)
	}
	return nil
}

func validateCorrectionNode(
	result *Result,
	rules CorrectionRuleRegistry,
	review ReviewObjective,
	finding ReviewFinding,
	correction CorrectionObjective,
	index int,
) error {
	rule, err := rules.rule(finding.FindingCode)
	if err != nil {
		return err
	}
	if finding.Kind != FindingSemanticIssue || correction.ID != correctionIdentity(correction) ||
		correction.RootObjectiveID != result.Objective.ID || correction.ParentID != review.ID ||
		!reflect.DeepEqual(correction.DependsOn, []cognitionreference.ObjectiveID{review.ID}) ||
		correction.Round != index+1 || correction.Finding.ID != finding.ID ||
		correction.InputArtifactID != review.ArtifactID || correction.InputSHA256 != review.ArtifactSHA256 ||
		correction.ObjectiveKind != rule.ObjectiveKind ||
		!reflect.DeepEqual(correction.Acceptance, rule.Acceptance) ||
		correction.OutputArtifactID == "" || correction.Status != ObjectiveComplete {
		return fmt.Errorf("%w: correction %d authority is invalid", ErrCompletion, index+1)
	}
	return nil
}

func validateCorrectionReceipt(
	root Objective,
	correction CorrectionObjective,
	receipt VerificationReceipt,
	index int,
) error {
	wantArtifactID := artifactIdentityFromAuthority(
		root.ID, correction.InputArtifactID, uint32(index+2), receipt.ArtifactSHA256,
	)
	if receipt.ID != verificationReceiptIdentity(receipt) ||
		receipt.Kind != VerificationCorrectionArtifact || receipt.RootObjectiveID != root.ID ||
		receipt.ArtifactID != wantArtifactID || receipt.ArtifactID != correction.OutputArtifactID ||
		receipt.ArtifactRevision != uint32(index+2) ||
		receipt.CorrectionObjectiveID != correction.ID ||
		!reflect.DeepEqual(receipt.ArtifactAcceptance, exactArtifactAcceptance) ||
		!reflect.DeepEqual(receipt.CorrectionAcceptance, correction.Acceptance) {
		return fmt.Errorf("%w: correction %d receipt is invalid", ErrCompletion, index+1)
	}
	return nil
}

func findingDefinition(
	specification ReviewSpecification,
	selected cognitionreference.CandidateID,
) (FindingDefinition, bool) {
	for _, definition := range specification.Candidates {
		if definition.CandidateID == selected {
			return definition, true
		}
	}
	return FindingDefinition{}, false
}
