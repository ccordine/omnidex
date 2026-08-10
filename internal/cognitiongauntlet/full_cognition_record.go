package cognitiongauntlet

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func sealFullCognitionExecution(
	ctx context.Context,
	fixture MicrogauntletCase,
	request FullCognitionRunRequest,
	authority PairedRunAuthority,
	episode cognition.EpisodeRef,
	components fullRuntimeComponents,
	run cognitionruntime.RunResult,
	restarts []RestartTrace,
) (SealedEpisode, error) {
	trace, err := readProductionTrace(ctx, components.repository, episode.ID)
	if err != nil {
		return SealedEpisode{}, err
	}
	template, err := fullCognitionEpisodeTemplate(fixture, episode, request, authority)
	if err != nil {
		return SealedEpisode{}, err
	}
	recorder, err := NewEpisodeRecorder(template)
	if err != nil {
		return SealedEpisode{}, err
	}
	recovery := RecoveryMetrics{Restarts: len(restarts)}
	metrics, err := appendProductionTrace(recorder, trace, recovery, restarts)
	if err != nil {
		return SealedEpisode{}, err
	}
	if metrics.Resources.ModelCalls != int(run.PolicyCalls) ||
		metrics.Resources.EnvironmentActions != int(run.EnvironmentActions) {
		return SealedEpisode{}, fmt.Errorf("production runtime counters differ from its sealed trace")
	}
	return recorder.Seal(
		request.EpisodeSealPath, trace.Header.Seal.FinalRevision, metrics.Outcome,
		metrics.Resources, metrics.Memory, metrics.Planning, metrics.Recovery,
	)
}
