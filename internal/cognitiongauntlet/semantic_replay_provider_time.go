package cognitiongauntlet

import "github.com/gryph/omnidex/internal/queue"

func semanticProviderBootstrapTimeExact(
	value queue.CognitionBrainBootstrapTrace,
	header queue.CognitionSealedTracePage,
) bool {
	if header.EpisodeStartedAt.IsZero() || header.SealedAt.IsZero() ||
		header.SealedAt.Before(header.EpisodeStartedAt) {
		return false
	}
	if value.Source == queue.CognitionBrainBootstrapEpisodeStart {
		return value.RecordedAt.Equal(header.EpisodeStartedAt)
	}
	return !value.RecordedAt.Before(header.EpisodeStartedAt) &&
		!value.RecordedAt.After(header.SealedAt)
}
