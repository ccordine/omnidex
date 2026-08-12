package cognitiongauntlet

import (
	"fmt"
	"reflect"
)

func verifyAblationSemanticAuthorities(
	bundle PublicInferenceBundle,
	episode SealedEpisode,
	evidence ablationEvidenceArtifact,
	bootstrapRaw []byte,
	activationRaw []byte,
	class AblationReplayClass,
) error {
	if err := validateAblationSemanticAuthority(bundle, episode); err != nil {
		return err
	}
	wantClass, private, err := ablationReplayClass(bundle.Authority.Variant)
	if err != nil || private || class != wantClass || evidence.Root.Class != wantClass {
		return fmt.Errorf("ablation semantic evidence class requires another replay boundary")
	}
	if err := verifyAblationEvidenceArtifact(evidence); err != nil {
		return err
	}
	evidenceRaw, err := encodeAblationEvidenceArtifact(evidence)
	if err != nil {
		return err
	}
	evidenceAuthority, err := ablationEvidenceAuthorityFromEpisode(episode)
	if err != nil || evidenceAuthority.SHA256 != digestExactBytes(evidenceRaw) ||
		evidenceAuthority.Bytes != len(evidenceRaw) {
		return fmt.Errorf("embedded ablation evidence differs from its sealed trace authority")
	}
	if evidence.Root.PublicRunAuthority != bundle.Authority ||
		evidence.Root.EpisodeID != episode.Manifest.EpisodeID ||
		!reflect.DeepEqual(evidence.Root.Goal, bundle.Goal) ||
		!reflect.DeepEqual(evidence.Root.Completion, bundle.Completion) ||
		!reflect.DeepEqual(evidence.Root.WorldCatalog, bundle.Catalog) ||
		evidence.Root.Terminal.Revision != episode.Manifest.FinalRevision ||
		evidence.Root.Terminal.PublicOutcome != episode.Manifest.Outcome.PublicOutcome ||
		evidence.Root.Terminal.GoalSatisfied != episode.Manifest.Outcome.GoalSatisfied ||
		evidence.Root.Terminal.FailureCode != episode.Manifest.Outcome.FailureCode ||
		!episode.Manifest.Outcome.Terminal {
		return fmt.Errorf("ablation semantic evidence differs from bundle or episode")
	}
	bootstrapAuthority, activationAuthority, err := ablationRuntimeAuthoritiesFromEpisode(episode)
	if err != nil || bootstrapAuthority != evidence.Root.BrainBootstrap ||
		activationAuthority != evidence.Root.ProviderActivation {
		return fmt.Errorf("ablation runtime identity differs from sealed trace authority")
	}
	frozen := bundle.Authority.RatGeneration.Fixed.Brain
	attested, err := frozen.attestedBrain()
	if err != nil {
		return err
	}
	bootstrap, err := decodeSemanticRuntimeBootstrap(bootstrapRaw, frozen)
	if err != nil {
		return err
	}
	activation, err := decodeSemanticRuntimeActivation(activationRaw, frozen)
	if err != nil {
		return err
	}
	if bootstrap.BootstrapEvidence.Ref != evidence.Root.BrainBootstrap.Evidence ||
		digestExactBytes(bootstrapRaw) != evidence.Root.BrainBootstrap.SHA256 ||
		activation.Receipt.ID != evidence.Root.ProviderActivation.ObservationID ||
		activation.IdentityEvidence.Ref != evidence.Root.ProviderActivation.Evidence ||
		digestExactBytes(activationRaw) != evidence.Root.ProviderActivation.SHA256 ||
		activation.Receipt.EpisodeID != evidence.Root.EpisodeID ||
		activation.Receipt.Actor != evidence.Root.Actor ||
		!sameFrozenBrain(bootstrap.AttestedBrain, attested) {
		return fmt.Errorf("ablation runtime identity differs from exact evidence authority")
	}
	if err := verifyAblationSemanticStateCausality(evidence.Root); err != nil {
		return err
	}
	if err := verifyAblationSemanticContextDerivation(evidence.Root); err != nil {
		return err
	}
	if err := verifyAblationSemanticEpisodeTrace(episode, evidence); err != nil {
		return err
	}
	return verifyAblationSemanticEpisodeResources(episode, evidence.Root)
}

func ablationRuntimeAuthoritiesFromEpisode(
	episode SealedEpisode,
) (RuntimeBrainBootstrapEvidenceAuthority, RuntimeProviderActivationEvidenceAuthority, error) {
	var bootstrap RuntimeBrainBootstrapEvidenceAuthority
	var activation RuntimeProviderActivationEvidenceAuthority
	bootstrapCount, activationCount := 0, 0
	for _, entry := range episode.Manifest.Trace {
		switch entry.Kind {
		case TraceProviderBootstrap:
			value, err := decodeRuntimeBrainBootstrapTrace(entry)
			if err != nil {
				return bootstrap, activation, err
			}
			bootstrap, bootstrapCount = value, bootstrapCount+1
		case TraceProviderActivation:
			value, err := decodeRuntimeProviderActivationTrace(entry)
			if err != nil {
				return bootstrap, activation, err
			}
			activation, activationCount = value, activationCount+1
		}
	}
	if bootstrapCount != 1 || activationCount != 1 {
		return bootstrap, activation, fmt.Errorf("sealed ablation runtime identity is incomplete")
	}
	return bootstrap, activation, nil
}
