package cognitiongauntlet

import "fmt"

type publicFullRuntimeEvidence struct {
	bootstrapArtifact   runtimeBrainBootstrapEvidenceArtifact
	bootstrapAuthority  RuntimeBrainBootstrapEvidenceAuthority
	activationArtifact  runtimeProviderActivationEvidenceArtifact
	activationAuthority RuntimeProviderActivationEvidenceAuthority
}

func preparePublicFullRuntimeEvidence(
	episodePath string,
	components fullRuntimeComponents,
) (publicFullRuntimeEvidence, error) {
	bootstrapArtifact, bootstrapAuthority, err := prepareRuntimeBrainBootstrapEvidence(
		episodePath, components.brainBootstrap, components.frozenFingerprint,
	)
	if err != nil {
		return publicFullRuntimeEvidence{}, err
	}
	activationArtifact, activationAuthority, err := prepareRuntimeProviderActivationEvidence(
		episodePath, components.providerActivation, components.frozenFingerprint,
	)
	if err != nil {
		return publicFullRuntimeEvidence{}, err
	}
	return publicFullRuntimeEvidence{
		bootstrapArtifact: bootstrapArtifact, bootstrapAuthority: bootstrapAuthority,
		activationArtifact: activationArtifact, activationAuthority: activationAuthority,
	}, nil
}

func (value publicFullRuntimeEvidence) appendTo(recorder *EpisodeRecorder) error {
	if recorder == nil {
		return fmt.Errorf("public Full runtime evidence recorder is nil")
	}
	if err := appendRuntimeBrainBootstrapTrace(recorder, value.bootstrapAuthority); err != nil {
		return err
	}
	return appendRuntimeProviderActivationTrace(recorder, value.activationAuthority)
}

func (value publicFullRuntimeEvidence) seal(
	episodePath string,
	frozen BrainFingerprint,
) error {
	if err := sealRuntimeBrainBootstrapEvidence(
		episodePath, value.bootstrapArtifact, value.bootstrapAuthority, frozen,
	); err != nil {
		return err
	}
	return sealRuntimeProviderActivationEvidence(
		episodePath, value.activationArtifact, value.activationAuthority, frozen,
	)
}
