package cognitiongauntlet

import "fmt"

func appendRuntimeBrainBootstrapTrace(
	recorder *EpisodeRecorder,
	authority RuntimeBrainBootstrapEvidenceAuthority,
) error {
	if err := authority.Validate(); err != nil {
		return err
	}
	payload, err := traceJSONObject(authority)
	if err != nil {
		return err
	}
	return recorder.Append(
		TraceProviderBootstrap,
		"runtime-brain-bootstrap-"+authority.SHA256,
		nil,
		payload,
	)
}

func decodeRuntimeBrainBootstrapTrace(
	entry TraceEntry,
) (RuntimeBrainBootstrapEvidenceAuthority, error) {
	var authority RuntimeBrainBootstrapEvidenceAuthority
	if entry.Kind != TraceProviderBootstrap || entry.Revision != nil ||
		decodeTracePayload(entry.Payload, &authority, "runtime Brain bootstrap trace") != nil ||
		authority.Validate() != nil || entry.ID != "runtime-brain-bootstrap-"+authority.SHA256 {
		return RuntimeBrainBootstrapEvidenceAuthority{},
			fmt.Errorf("runtime Brain bootstrap trace authority is invalid")
	}
	return authority, nil
}

func verifySealedEpisodeRuntimeProviderIdentity(
	episodePath string,
	seal SealedEpisode,
) error {
	var authority RuntimeBrainBootstrapEvidenceAuthority
	var activationAuthority RuntimeProviderActivationEvidenceAuthority
	bootstrapCount, activationCount := 0, 0
	bootstrapSequence, activationSequence := uint64(0), uint64(0)
	for _, entry := range seal.Manifest.Trace {
		switch entry.Kind {
		case TraceProviderBootstrap:
			decoded, err := decodeRuntimeBrainBootstrapTrace(entry)
			if err != nil {
				return err
			}
			authority = decoded
			bootstrapCount++
			bootstrapSequence = entry.Sequence
		case TraceProviderActivation:
			decoded, err := decodeRuntimeProviderActivationTrace(entry)
			if err != nil {
				return err
			}
			activationAuthority = decoded
			activationCount++
			activationSequence = entry.Sequence
		}
	}
	if executableAblation(seal.Manifest.Variant) &&
		(bootstrapCount != 1 || activationCount != 1) {
		return fmt.Errorf("sealed cognition ablation requires exact runtime Brain bootstrap and provider activation evidence")
	}
	if bootstrapCount == 0 && activationCount == 0 {
		return nil
	}
	if bootstrapCount != 1 || activationCount != 1 ||
		bootstrapSequence >= activationSequence ||
		seal.Manifest.Variant == VariantDeterministicOracle {
		return fmt.Errorf("sealed cognition episode runtime provider identity evidence is invalid")
	}
	if _, err := loadRuntimeBrainBootstrapEvidence(
		episodePath, authority, seal.Manifest.RatGeneration.Fixed.Brain,
	); err != nil {
		return err
	}
	_, err := loadRuntimeProviderActivationEvidence(
		episodePath, activationAuthority, seal.Manifest.RatGeneration.Fixed.Brain,
	)
	return err
}
