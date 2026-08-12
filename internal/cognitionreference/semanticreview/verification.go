package semanticreview

import (
	"context"
	"fmt"
)

var exactArtifactAcceptance = []AcceptancePredicate{AcceptanceCurrentArtifactVerified}

func verifyCurrent(
	ctx context.Context,
	verifier Verifier,
	root Objective,
	artifact Artifact,
	correction *CorrectionObjective,
) (VerificationReceipt, error) {
	if err := ctx.Err(); err != nil {
		return VerificationReceipt{}, err
	}
	input := VerificationInput{
		Kind: VerificationCurrentArtifact, RootObjectiveID: root.ID,
		Artifact:             cloneArtifact(artifact),
		ArtifactAcceptance:   append([]AcceptancePredicate{}, exactArtifactAcceptance...),
		CorrectionAcceptance: []CorrectionAcceptancePredicate{},
	}
	if correction != nil {
		owned := cloneCorrectionObjective(*correction)
		input.Kind = VerificationCorrectionArtifact
		input.Correction = &owned
		input.CorrectionAcceptance = append([]CorrectionAcceptancePredicate{}, owned.Acceptance...)
	}
	if err := verifier.Verify(ctx, cloneVerificationInput(input)); err != nil {
		return VerificationReceipt{}, fmt.Errorf("%w: %v", ErrVerification, err)
	}
	if err := ctx.Err(); err != nil {
		return VerificationReceipt{}, err
	}
	receipt := VerificationReceipt{
		Kind: input.Kind, RootObjectiveID: root.ID, ArtifactID: artifact.ID,
		ArtifactSHA256: artifact.SHA256, ArtifactRevision: artifact.Revision,
		ArtifactAcceptance:   append([]AcceptancePredicate{}, input.ArtifactAcceptance...),
		CorrectionAcceptance: append([]CorrectionAcceptancePredicate{}, input.CorrectionAcceptance...),
	}
	if correction != nil {
		receipt.CorrectionObjectiveID = correction.ID
	}
	receipt.ID = verificationReceiptIdentity(receipt)
	return cloneVerificationReceipt(receipt), nil
}

func verificationReceiptIdentity(receipt VerificationReceipt) VerificationReceiptID {
	fields := []string{
		string(receipt.Kind), string(receipt.RootObjectiveID), string(receipt.ArtifactID),
		receipt.ArtifactSHA256, fmt.Sprintf("%d", receipt.ArtifactRevision),
		string(receipt.CorrectionObjectiveID),
	}
	for _, predicate := range receipt.ArtifactAcceptance {
		fields = append(fields, string(predicate))
	}
	for _, predicate := range receipt.CorrectionAcceptance {
		fields = append(fields, string(predicate))
	}
	return VerificationReceiptID("V" + digestFields(fields...))
}

func cloneVerificationInput(value VerificationInput) VerificationInput {
	value.Artifact = cloneArtifact(value.Artifact)
	value.ArtifactAcceptance = append([]AcceptancePredicate{}, value.ArtifactAcceptance...)
	value.CorrectionAcceptance = append([]CorrectionAcceptancePredicate{}, value.CorrectionAcceptance...)
	if value.Correction != nil {
		owned := cloneCorrectionObjective(*value.Correction)
		value.Correction = &owned
	}
	return value
}

func cloneVerificationReceipt(value VerificationReceipt) VerificationReceipt {
	value.ArtifactAcceptance = append([]AcceptancePredicate{}, value.ArtifactAcceptance...)
	value.CorrectionAcceptance = append([]CorrectionAcceptancePredicate{}, value.CorrectionAcceptance...)
	return value
}
