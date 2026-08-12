package cognitiongauntlet

import (
	"fmt"
	"reflect"
	"time"

	"github.com/gryph/omnidex/internal/cognition"
)

func validateSemanticEpisodeDerivedAuthority(
	episode SealedEpisode,
	trace productionTrace,
) error {
	template := episode.Manifest
	template.EpisodeStartedAt = time.Time{}
	template.SealedAt = time.Time{}
	template.FinalRevision = cognition.WorldRevision{}
	template.Outcome = Outcome{}
	template.TraceSHA256 = ""
	template.Trace = nil
	template.Resources = Resources{}
	template.Memory = MemoryMetrics{}
	template.Planning = PlanningMetrics{}
	template.Recovery = RecoveryMetrics{}
	recorder, err := NewEpisodeRecorder(template)
	if err != nil {
		return fmt.Errorf("rebuild semantic episode recorder: %w", err)
	}
	bootstrap, activation, err := semanticReplayRuntimeEvidenceAuthorities(episode)
	if err != nil {
		return err
	}
	if err := appendRuntimeBrainBootstrapTrace(recorder, bootstrap); err != nil {
		return fmt.Errorf("rebuild runtime Brain bootstrap trace: %w", err)
	}
	if err := appendRuntimeProviderActivationTrace(recorder, activation); err != nil {
		return fmt.Errorf("rebuild runtime provider activation trace: %w", err)
	}
	metrics, err := appendProductionTrace(
		recorder, trace, RecoveryMetrics{}, nil,
	)
	if err != nil {
		return fmt.Errorf("rebuild semantic episode trace: %w", err)
	}
	manifest := episode.Manifest
	if !reflect.DeepEqual(recorder.trace, manifest.Trace) ||
		metrics.Resources != manifest.Resources ||
		metrics.Memory != manifest.Memory ||
		metrics.Planning != manifest.Planning ||
		metrics.Recovery != manifest.Recovery ||
		metrics.Outcome != manifest.Outcome {
		return fmt.Errorf("semantic episode trace or derived metrics differ from exact queue records")
	}
	return nil
}

func semanticReplayRuntimeEvidenceAuthorities(
	episode SealedEpisode,
) (
	RuntimeBrainBootstrapEvidenceAuthority,
	RuntimeProviderActivationEvidenceAuthority,
	error,
) {
	if len(episode.Manifest.Trace) < 2 {
		return RuntimeBrainBootstrapEvidenceAuthority{},
			RuntimeProviderActivationEvidenceAuthority{},
			fmt.Errorf("semantic Full episode lacks runtime provider evidence trace authorities")
	}
	bootstrap, err := decodeRuntimeBrainBootstrapTrace(episode.Manifest.Trace[0])
	if err != nil {
		return RuntimeBrainBootstrapEvidenceAuthority{},
			RuntimeProviderActivationEvidenceAuthority{}, err
	}
	activation, err := decodeRuntimeProviderActivationTrace(episode.Manifest.Trace[1])
	if err != nil {
		return RuntimeBrainBootstrapEvidenceAuthority{},
			RuntimeProviderActivationEvidenceAuthority{}, err
	}
	for _, entry := range episode.Manifest.Trace[2:] {
		if entry.Kind == TraceProviderBootstrap || entry.Kind == TraceProviderActivation {
			return RuntimeBrainBootstrapEvidenceAuthority{},
				RuntimeProviderActivationEvidenceAuthority{},
				fmt.Errorf("semantic Full episode duplicates runtime provider evidence authority")
		}
	}
	return bootstrap, activation, nil
}
