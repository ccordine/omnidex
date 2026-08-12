package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreplay"
)

func readProductionSemanticReplayEvidence(
	ctx context.Context,
	reader ProductionSemanticReplayReader,
	bundle PublicInferenceBundle,
	trace productionTrace,
	runtimeSidecars ProductionSemanticReplaySidecars,
) (semanticReplaySupplement, error) {
	frozen, err := bundle.Authority.RatGeneration.Fixed.Brain.attestedBrain()
	if err != nil {
		return semanticReplaySupplement{}, err
	}
	inventory, err := collectSemanticReplayEvidence(trace, frozen)
	if err != nil {
		return semanticReplaySupplement{}, err
	}
	if err := preflightProductionSemanticEvidence(trace, inventory, runtimeSidecars); err != nil {
		return semanticReplaySupplement{}, err
	}
	var supplement semanticReplaySupplement
	if err := readSemanticPolicyBodies(
		ctx, reader, trace.Header.EpisodeID, trace, inventory, &supplement,
	); err != nil {
		return semanticReplaySupplement{}, err
	}
	identities, err := readSemanticProviderIdentities(
		ctx, reader, trace.Header.EpisodeID, inventory, &supplement,
	)
	if err != nil {
		return semanticReplaySupplement{}, err
	}
	if err := supplement.setIdentities(identities); err != nil {
		return semanticReplaySupplement{}, err
	}
	if err := addSemanticRuntimeSidecars(
		runtimeSidecars, bundle.Authority.RatGeneration.Fixed.Brain,
		inventory, identities, &supplement,
	); err != nil {
		return semanticReplaySupplement{}, err
	}
	if err := supplement.finish(); err != nil {
		return semanticReplaySupplement{}, err
	}
	return supplement, nil
}

func preflightProductionSemanticEvidence(
	trace productionTrace,
	inventory semanticReplayEvidenceInventory,
	runtimeSidecars ProductionSemanticReplaySidecars,
) error {
	if err := runtimeSidecars.validate(); err != nil {
		return err
	}
	remaining := cognitionreplay.MaxContainerBytes
	consume := func(size int) error {
		if size < 0 || size > remaining {
			return fmt.Errorf("semantic replay evidence exceeds its container bound")
		}
		remaining -= size
		return nil
	}
	for _, record := range trace.Records {
		if err := consume(len(record.Payload)); err != nil {
			return err
		}
	}
	for _, metadata := range inventory.policy {
		if err := validateSemanticPolicyEvidenceBytes(metadata); err != nil {
			return err
		}
		if err := consume(metadata.Bytes); err != nil {
			return err
		}
	}
	for _, ref := range inventory.identities {
		if err := ref.Validate(); err != nil {
			return err
		}
		if err := consume(ref.Bytes); err != nil {
			return err
		}
	}
	if err := consume(len(runtimeSidecars.RuntimeBrainBootstrapEvidence)); err != nil {
		return err
	}
	return consume(len(runtimeSidecars.RuntimeProviderActivationEvidence))
}
